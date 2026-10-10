package agent

import "testing"

func TestToolStatusFromHistoryContentListSkillsOK(t *testing.T) {
	// 技能描述里常有 error 字样；旧逻辑会误标失败
	body := `{"ok":true,"data":"[{\"id\":\"container-inspect\",\"description\":\"handle error logs\"}]"}` + "\n〔resource#12〕"
	st, _ := toolStatusFromHistoryContent("list_skills", body)
	if st != StatusOK {
		t.Fatalf("status=%q want ok", st)
	}
}

func TestToolStatusFromHistoryContentRealFail(t *testing.T) {
	st, sum := toolStatusFromHistoryContent("list_skills", `{"ok":false,"error":"该能力已关闭"}`)
	if st != StatusError {
		t.Fatalf("status=%q want error", st)
	}
	if sum != "该能力已关闭" {
		t.Fatalf("summary=%q", sum)
	}
}
