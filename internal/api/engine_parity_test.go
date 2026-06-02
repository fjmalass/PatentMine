package api_test

import (
	"testing"

	"patentmine/internal/command"
	"patentmine/internal/proto"
)

// engineMethodsWithREST lists proto methods that must have a REST route for web/TUI parity.
var engineMethodsWithREST = map[proto.Method]bool{
	proto.MethodPing:                    true, // /healthz
	proto.MethodPatentList:              true,
	proto.MethodPatentGet:               true,
	proto.MethodPatentDelete:            true,
	proto.MethodPatentDeleteBulk:        true,
	proto.MethodPatentClearCache:        true,
	proto.MethodOrphanList:              true,
	proto.MethodPatentTableColumns:      true,
	proto.MethodRelations:               true,
	proto.MethodFamilyGraph:             true,
	proto.MethodReviewState:             true,
	proto.MethodMembershipAdd:           true,
	proto.MethodAddRelated:              true,
	proto.MethodProjectList:             true,
	proto.MethodProjectCreate:           true,
	proto.MethodProjectUpdate:           true,
	proto.MethodIDSExport:               true,
	proto.MethodIDSPDFExport:            true,
	proto.MethodIDSPDFPreview:           true,
	proto.MethodIDSEntryGet:             true,
	proto.MethodIDSEntrySave:            true,
	proto.MethodIDSEntryDelete:          true,
	proto.MethodIDSEntryBulkSetStatus:   true,
	proto.MethodPatentNoteGet:           true,
	proto.MethodPatentNoteSave:          true,
	proto.MethodPatentNoteDelete:        true,
	proto.MethodPatentNoteList:          true,
	proto.MethodPatentNoteExport:        true,
	proto.MethodCrawlFamily:             true,
	proto.MethodCrawlCancel:             true,
	proto.MethodCrawlConfig:             true,
	proto.MethodImportFile:              true,
	proto.MethodSourceModeGet:           true,
	proto.MethodSourceModeSet:           true,
	proto.MethodTagCreate:               true,
	proto.MethodTagList:                 true,
	proto.MethodTagDelete:               true,
	proto.MethodTagPatent:               true,
	proto.MethodUntagPatent:             true,
	proto.MethodTagPatentStrict:         true,
	proto.MethodUntagPatentStrict:       true,
	proto.MethodPatentTagList:           true,
	proto.MethodClassificationList:      true,
	proto.MethodClassificationGet:       true,
	proto.MethodClassificationSave:      true,
	proto.MethodClassificationDelete:    true,
	proto.MethodClassificationLookup:    true,
	proto.MethodClassificationListByCodes: true,
	proto.MethodPatentClassificationList: true,
	proto.MethodSourceDiffsList:         true,
	proto.MethodSourceResolveDiffs:      true,
	proto.MethodPatentInventorStats:     true,
	proto.MethodSourceBibsList:          true,
	proto.MethodPatentAssigneeStats:     true,
	proto.MethodPatentClassificationStats: true,
	proto.MethodProjectAssignees:        true,
	proto.MethodUSPTOFetchXML:           true,
	proto.MethodUSPTOGrantBody:          true,
	proto.MethodUSPTOFetchAssignments:   true,
	proto.MethodUSPTOAssignmentList:     true,
	proto.MethodUSPTOFetchAssignmentsBatch: true,
	proto.MethodAssigneeHistory:         true,
	proto.MethodUSPTOExpirationCalculate: true,
	proto.MethodAddedExport:             true,
	proto.MethodAddedImport:             true,
	proto.MethodMetricsGet:              true,
	proto.MethodMetricsPush:             true,
	proto.MethodTableViewList:           true,
	proto.MethodTableViewGet:            true,
	proto.MethodTableViewSave:           true,
	proto.MethodTableViewDelete:         true,
}

func TestEngineCommandsHaveRESTCoverage(t *testing.T) {
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	var missing []string
	for _, cmd := range reg.All() {
		if cmd.Kind != command.KindEngine {
			continue
		}
		if cmd.Method == "" {
			continue
		}
		if !engineMethodsWithREST[cmd.Method] {
			missing = append(missing, string(cmd.ID)+" -> "+string(cmd.Method))
		}
	}
	if len(missing) > 0 {
		t.Fatalf("KindEngine commands without REST mapping:\n  %v", missing)
	}
}
