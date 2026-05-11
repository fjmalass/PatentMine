#!/bin/bash

# This script verifies the USPTO ODP API configuration.

if [ -z "$1" ]; then
    echo "Usage: $0 <patent-number> [api-key]"
    exit 1
fi

PATENT_NUM=$1
API_KEY=$2

# Try to find API key in config if not provided
if [ -z "$API_KEY" ]; then
    CONFIG_FILE="$HOME/.config/patentmine/config.toml"
    if [ -f "$CONFIG_FILE" ]; then
        API_KEY=$(grep "api_key =" "$CONFIG_FILE" | head -n1 | cut -d'=' -f2 | tr -d ' "' )
    fi
fi

if [ -z "$API_KEY" ]; then
    echo "Error: No USPTO API key provided and none found in $CONFIG_FILE"
    echo "Register at developer.uspto.gov and add to config.toml or pass as second argument."
    exit 1
fi

echo "Testing USPTO ODP API for patent $PATENT_NUM..."
SEARCH_NUM=$(echo "$PATENT_NUM" | sed 's/^US//' | sed 's/[A-Z][0-9]$//')

QUERY="patentNumber:($SEARCH_NUM) OR publicationSequenceNumberBag:($PATENT_NUM)"
ENCODED_QUERY=$(echo "$QUERY" | sed 's/ /%20/g' | sed 's/(/%28/g' | sed 's/)/%29/g' | sed 's/:/%3A/g')

URL="https://api.uspto.gov/api/v1/patent/applications/search?q=$ENCODED_QUERY"

if [ "$PATENT_DEBUG" == "1" ]; then
    echo ">>> REQ: $URL"
fi

RESPONSE=$(curl -s -H "X-API-KEY: $API_KEY" "$URL")

if [ "$PATENT_DEBUG" == "1" ]; then
    echo "<<< RESP: $RESPONSE"
fi

if echo "$RESPONSE" | grep -q "patentFileWrapperDataBag"; then
    COUNT=$(echo "$RESPONSE" | grep -o '"count":[0-9]*' | cut -d':' -f2)
    echo "Success! Found $COUNT matching application(s) for $PATENT_NUM."
    
    if [ "$COUNT" -gt "0" ]; then
        # Check forward citations
        echo "Checking forward citations..."
        FWD_URL="https://api.uspto.gov/api/v1/patent/applications/search?q=forwardReferencedPatentNumber%3A%28$SEARCH_NUM%29&rows=0"
        if [ "$PATENT_DEBUG" == "1" ]; then
             echo ">>> REQ: $FWD_URL"
        fi
        FWD_RESPONSE=$(curl -s -H "X-API-KEY: $API_KEY" "$FWD_URL")
        if [ "$PATENT_DEBUG" == "1" ]; then
             echo "<<< RESP: $FWD_RESPONSE"
        fi
        FWD_COUNT=$(echo "$FWD_RESPONSE" | grep -o '"count":[0-9]*' | cut -d':' -f2)
        echo "Forward citations count: $FWD_COUNT"
    fi
else
    echo "Error: API request failed or patent not found."
    echo "Response: $RESPONSE"
fi
