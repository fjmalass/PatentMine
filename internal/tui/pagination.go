package tui

import "fmt"

type pageWindowInfo struct {
	Start int
	End   int
	Page  int
	Pages int
	Total int
}

func pageWindow(selected, total, pageSize int) pageWindowInfo {
	if total <= 0 {
		return pageWindowInfo{}
	}
	if pageSize <= 0 {
		pageSize = total
	}
	selected = clamp(selected, 0, total-1)
	start := pageStart(selected, pageSize)
	end := min(start+pageSize, total)
	return pageWindowInfo{
		Start: start,
		End:   end,
		Page:  start/pageSize + 1,
		Pages: (total + pageSize - 1) / pageSize,
		Total: total,
	}
}

func pageStart(selected, pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	return (selected / pageSize) * pageSize
}

func pageStatus(format string, window pageWindowInfo) string {
	return fmt.Sprintf(format, window.Page, window.Pages, window.Start+1, window.End, window.Total)
}
