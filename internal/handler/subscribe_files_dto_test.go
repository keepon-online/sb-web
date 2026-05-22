package handler

import (
	"reflect"
	"testing"

	"miaomiaowu/internal/storage"
)

func TestParseExpireAt_Nil(t *testing.T) {
	got, err := parseExpireAt(nil)
	if err != nil {
		t.Fatalf("nil input unexpected err: %v", err)
	}
	if got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
}

func TestParseExpireAt_EmptyString(t *testing.T) {
	empty := ""
	got, err := parseExpireAt(&empty)
	if err != nil {
		t.Fatalf("empty input unexpected err: %v", err)
	}
	if got != nil {
		t.Errorf("empty string should return nil, got %v", got)
	}
	whitespace := "   "
	got, err = parseExpireAt(&whitespace)
	if err != nil || got != nil {
		t.Errorf("whitespace should return nil/nil, got %v/%v", got, err)
	}
}

func TestParseExpireAt_RFC3339(t *testing.T) {
	in := "2026-05-21T10:00:00Z"
	got, err := parseExpireAt(&in)
	if err != nil {
		t.Fatalf("RFC3339 unexpected err: %v", err)
	}
	if got == nil {
		t.Fatal("RFC3339 should return non-nil time")
	}
	if got.Year() != 2026 || got.Month() != 5 || got.Day() != 21 {
		t.Errorf("parsed date wrong: %v", got)
	}
}

func TestParseExpireAt_RFC3339Nano(t *testing.T) {
	in := "2026-05-21T10:00:00.123456789Z"
	got, err := parseExpireAt(&in)
	if err != nil {
		t.Fatalf("RFC3339Nano unexpected err: %v", err)
	}
	if got == nil {
		t.Fatal("RFC3339Nano should return non-nil time")
	}
}

func TestParseExpireAt_RejectsGarbage(t *testing.T) {
	in := "not a date"
	if _, err := parseExpireAt(&in); err == nil {
		t.Error("garbage input should error")
	}
}

func TestConvertSubscribeFile_NilSelectedTags(t *testing.T) {
	file := storage.SubscribeFile{
		ID:       42,
		Name:     "test",
		Filename: "test.yaml",
		// SelectedTags intentionally nil
	}
	dto := convertSubscribeFile(file)
	if dto.SelectedTags == nil {
		t.Error("SelectedTags should be normalized to empty slice, not nil")
	}
	if len(dto.SelectedTags) != 0 {
		t.Errorf("SelectedTags should be empty, got %v", dto.SelectedTags)
	}
	if dto.ID != 42 || dto.Name != "test" {
		t.Errorf("field copy mismatch: %+v", dto)
	}
}

func TestConvertSubscribeFile_PreservesAllFields(t *testing.T) {
	limit := 100.5
	file := storage.SubscribeFile{
		ID:                  7,
		Name:                "n",
		Description:         "d",
		Type:                "create",
		Filename:            "f.yaml",
		AutoSyncCustomRules: true,
		TemplateFilename:    "tpl.yaml",
		SelectedTags:        []string{"a", "b"},
		CustomShortCode:     "code",
		RawOutput:           true,
		TrafficLimit:        &limit,
	}
	dto := convertSubscribeFile(file)
	if dto.AutoSyncCustomRules != true {
		t.Error("AutoSyncCustomRules lost")
	}
	if dto.TemplateFilename != "tpl.yaml" {
		t.Error("TemplateFilename lost")
	}
	if !reflect.DeepEqual(dto.SelectedTags, []string{"a", "b"}) {
		t.Errorf("SelectedTags lost: %v", dto.SelectedTags)
	}
	if dto.CustomShortCode != "code" || !dto.RawOutput {
		t.Errorf("scalar field lost: %+v", dto)
	}
	if dto.TrafficLimit == nil || *dto.TrafficLimit != 100.5 {
		t.Errorf("TrafficLimit lost: %v", dto.TrafficLimit)
	}
}

func TestConvertSubscribeFiles_Batch(t *testing.T) {
	files := []storage.SubscribeFile{
		{ID: 1, Name: "a"},
		{ID: 2, Name: "b"},
		{ID: 3, Name: "c"},
	}
	out := convertSubscribeFiles(files)
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	for i, d := range out {
		if d.ID != int64(i+1) {
			t.Errorf("dto[%d].ID = %d, want %d", i, d.ID, i+1)
		}
	}
}

func TestConvertSubscribeFiles_EmptyInput(t *testing.T) {
	out := convertSubscribeFiles(nil)
	if out == nil {
		t.Error("nil input should yield non-nil empty slice")
	}
	if len(out) != 0 {
		t.Errorf("len = %d, want 0", len(out))
	}
}
