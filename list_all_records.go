package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "/home/francois/.config/patentmine/patentmine.db")
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT p.number, u.application_number, u.invention_title, u.computed_expiration_date
		FROM uspto_application u
		LEFT JOIN patent p ON p.record_id = u.record_id
	`)
	if err != nil {
		log.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	fmt.Println("All USPTO Applications in DB:")
	for rows.Next() {
		var patNum, appNum, title, expDate sql.NullString
		if err := rows.Scan(&patNum, &appNum, &title, &expDate); err != nil {
			log.Fatal(err)
		}
		fmt.Printf(" - Patent: %s, App: %s, Title: %s, Exp: %s\n",
			val(patNum), val(appNum), val(title), val(expDate))
	}
}

func val(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return "NULL"
}
