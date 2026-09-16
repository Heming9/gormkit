package gormkit

import (
	"testing"

	"gorm.io/gorm"
)

func TestTenantControlStatement(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want bool
	}{
		{name: "savepoint", sql: "SAVEPOINT sp123", want: true},
		{name: "rollback", sql: "ROLLBACK TO SAVEPOINT sp123", want: true},
		{name: "release", sql: "RELEASE SAVEPOINT sp123", want: true},
		{name: "arbitrary name", sql: "SAVEPOINT custom", want: false},
		{name: "missing identifier", sql: "SAVEPOINT ", want: false},
		{name: "injected suffix", sql: "SAVEPOINT sp123; DELETE FROM tenant_records", want: false},
		{name: "data statement", sql: "UPDATE tenant_records SET name = 'unsafe'", want: false},
		{name: "lowercase", sql: "savepoint sp123", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statement := &gorm.Statement{}
			statement.SQL.WriteString(test.sql)
			if got := isTenantControlStatement(statement); got != test.want {
				t.Fatalf("isTenantControlStatement(%q) = %v, want %v", test.sql, got, test.want)
			}
		})
	}

	statement := &gorm.Statement{Vars: []any{"sp123"}}
	statement.SQL.WriteString("SAVEPOINT ?")
	if isTenantControlStatement(statement) {
		t.Fatal("parameterized transaction statement was allowed")
	}
}
