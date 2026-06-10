package data

import (
	pkgsql "github.com/openmodu/oneshot/pkg/sql"
)

type Data struct {
	Sql *pkgsql.Sql
}

func NewDataWithSQL(sql *pkgsql.Sql) *Data {
	return &Data{Sql: sql}
}

func (d *Data) Close() error {
	if d == nil || d.Sql == nil {
		return nil
	}
	return d.Sql.Close()
}
