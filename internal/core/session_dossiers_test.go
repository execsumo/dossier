package core_test

import (
	"reflect"
	"testing"
	"time"

	"dossier/internal/core"
	"dossier/internal/store"
)

func TestSessionDossiers(t *testing.T) {
	st := store.NewFakeStore()
	st.Sessions["s1"] = &core.SessionBinding{SessionBindingID: "s1", DossierID: "dos_1"}
	st.Sessions["s2"] = &core.SessionBinding{SessionBindingID: "s2", DossierID: "dos_2"}
	st.Sessions["cleared"] = &core.SessionBinding{SessionBindingID: "cleared"}
	svc := core.NewService(st, nil, attentionTokenizer{}, nil, &attentionClock{now: time.Now()}, core.Config{}, nil)

	got := svc.SessionDossiers([]string{"s1", "s2", "cleared", "missing", ""})
	want := map[string]string{"s1": "dos_1", "s2": "dos_2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SessionDossiers = %v, want %v", got, want)
	}
}
