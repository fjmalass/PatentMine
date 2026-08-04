package command

import "patentmine/internal/proto"

// projectCommands returns the projectCommands command registrations.
func projectCommands() []Command {
	return []Command{
		Command{ID: ProjectCreate, Name: "project.create", Usage: ":project.create [NAME]", Kind: KindEngine, Method: proto.MethodProjectCreate, Scopes: []Scope{ScopeProjects}},
		Command{ID: ProjectIDSHeader, Name: "project.ids-header", Aliases: []string{"ids-header", "project-header"}, Usage: ":project.ids-header", Kind: KindView, Scopes: projectScopes},
		Command{ID: IDSExportPDF, Name: "export.ids.pdf", Aliases: []string{"ids-pdf", "ids.export.pdf"}, Usage: ":export.ids.pdf", Kind: KindEngine, Method: proto.MethodIDSPDFExport, Scopes: projectScopes},
	}
}
