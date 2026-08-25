package gormkit_test

import (
	"errors"
	"testing"

	"github.com/Heming9/gormkit"
)

func TestJSONDataPreservesValueOnMarshalError(t *testing.T) {
	data := gormkit.JSONData(`{"ok":true}`)
	err := data.Marshal(make(chan int))
	if err == nil {
		t.Fatal("expected marshal error")
	}
	if string(data) != `{"ok":true}` {
		t.Fatalf("marshal error changed data: %s", data)
	}
	var decoded struct {
		OK bool `json:"ok"`
	}
	if err := data.Unmarshal(&decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.OK {
		t.Fatal("JSONData did not unmarshal")
	}
}

func TestRepositoryRejectsNonPointerModel(t *testing.T) {
	database := openTestDatabase(t)
	repo := gormkit.NewRepository[testUser](database.Client(nil))
	if _, err := repo.DB(); !errors.Is(err, gormkit.ErrInvalidModel) {
		t.Fatalf("expected ErrInvalidModel, got %v", err)
	}
}
