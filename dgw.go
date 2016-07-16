// go:generate go-bindata -o bindata.go template mapconfig
package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"go/format"
	"html/template"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/achiku/varfmt"
	"github.com/pkg/errors"
)

const pgLoadColumnDef = `
SELECT
    a.attnum AS field_ordinal,
    a.attname AS column_name,
    format_type(a.atttypid, a.atttypmod) AS data_type,
    a.attnotnull AS not_null,
    COALESCE(pg_get_expr(ad.adbin, ad.adrelid), '') AS default_value,
    COALESCE(ct.contype = 'p', false) AS  is_primary_key
FROM pg_attribute a
JOIN ONLY pg_class c ON c.oid = a.attrelid
JOIN ONLY pg_namespace n ON n.oid = c.relnamespace
LEFT JOIN pg_constraint ct ON ct.conrelid = c.oid
AND a.attnum = ANY(ct.conkey) AND ct.contype IN ('p', 'u')
LEFT JOIN pg_attrdef ad ON ad.adrelid = c.oid AND ad.adnum = a.attnum
WHERE a.attisdropped = false
AND n.nspname = $1
AND c.relname = $2
AND a.attnum > 0
ORDER BY a.attnum
`

const pgLoadTableDef = `
SELECT
c.relkind AS type,
c.relname AS table_name
FROM pg_class c
JOIN ONLY pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1
AND c.relkind = 'r'
`

// TypeMap go/db type map struct
type TypeMap struct {
	DBTypes          []string `toml:"db_types"`
	NotNullGoType    string   `toml:"notnull_go_type"`
	NotNullNilValue  string   `toml:"notnull_nil_value"`
	NullableGoType   string   `toml:"nullable_go_type"`
	NullableNilValue string   `toml:"nullable_nil_value"`
}

// PgTypeMapConfig go/db type map struct toml config
type PgTypeMapConfig map[string]TypeMap

// PgTable postgres table
type PgTable struct {
	Schema   string
	Name     string
	DataType string
	Columns  []*PgColumn
}

// PgColumn postgres columns
type PgColumn struct {
	FieldOrdinal int
	Name         string
	DataType     string
	NotNull      bool
	DefaultValue sql.NullString
	IsPrimaryKey bool
}

// PgLoadTypeMapFromFile load type map from toml file
func PgLoadTypeMapFromFile(filePath string) (*PgTypeMapConfig, error) {
	var conf PgTypeMapConfig
	if _, err := toml.DecodeFile(filePath, &conf); err != nil {
		return nil, errors.Wrap(err, "faild to parse config file")
	}
	return &conf, nil
}

// PgLoadColumnDef load Postgres column definition
func PgLoadColumnDef(db Queryer, schema string, table string) ([]*PgColumn, error) {
	colDefs, err := db.Query(pgLoadColumnDef, schema, table)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load table def")
	}

	cols := []*PgColumn{}
	for colDefs.Next() {
		c := &PgColumn{}
		err := colDefs.Scan(
			&c.FieldOrdinal,
			&c.Name,
			&c.DataType,
			&c.NotNull,
			&c.DefaultValue,
			&c.IsPrimaryKey,
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan")
		}
		cols = append(cols, c)
	}
	return cols, nil
}

// PgLoadTableDef load Postgres table definition
func PgLoadTableDef(db Queryer, schema string) ([]*PgTable, error) {
	tbDefs, err := db.Query(pgLoadTableDef, schema)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load table def")
	}
	tbs := []*PgTable{}
	for tbDefs.Next() {
		t := &PgTable{Schema: schema}
		err := tbDefs.Scan(
			&t.DataType,
			&t.Name,
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan")
		}
		cols, err := PgLoadColumnDef(db, schema, t.Name)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("failed to get columns of %s", t.Name))
		}
		t.Columns = cols
		tbs = append(tbs, t)
	}
	return tbs, nil
}

// StructField go struct field
type StructField struct {
	Name   string
	Type   string
	Tag    string
	NilVal string
	Col    *PgColumn
}

// Struct go struct
type Struct struct {
	Name      string
	TableName string
	Schema    string
	Comment   string
	Fields    []*StructField
}

func contains(v string, l []string) bool {
	sort.Strings(l)
	i := sort.SearchStrings(l, v)
	if i < len(l) && l[i] == v {
		return true
	}
	return false
}

// PgConvertType converts type
func PgConvertType(col *PgColumn, typeCfg *PgTypeMapConfig) (string, string) {
	cfg := map[string]TypeMap(*typeCfg)
	typ := cfg["default"].NotNullGoType
	nilVal := cfg["default"].NotNullNilValue
	for _, v := range cfg {
		if contains(col.DataType, v.DBTypes) {
			if col.NotNull {
				return v.NotNullGoType, v.NotNullNilValue
			}
			return v.NullableGoType, v.NullableNilValue
		}
	}
	return typ, nilVal
}

// PgColToField converts pg column to go struct field
func PgColToField(col *PgColumn, typeCfg *PgTypeMapConfig) (*StructField, error) {
	stfName := varfmt.PublicVarName(col.Name)
	stfType, nilVal := PgConvertType(col, typeCfg)
	stf := &StructField{Name: stfName, Type: stfType, NilVal: nilVal, Col: col}
	return stf, nil
}

const structTmpl = `
// {{ .Name }} represents {{ .Schema }}.{{ .TableName }}
type {{ .Name }} struct {
{{- range .Fields }}
	{{ .Name }} {{ .Type }} // {{ .Col.Name }}
{{- end }}
}`

// PgTableToStruct converts table def to go struct
func PgTableToStruct(t *PgTable, typeCfg *PgTypeMapConfig) (*Struct, error) {
	s := &Struct{
		Name:      varfmt.PublicVarName(t.Name),
		TableName: t.Name,
		Schema:    t.Schema,
	}
	var fs []*StructField
	for _, c := range t.Columns {
		f, err := PgColToField(c, typeCfg)
		if err != nil {
			return nil, errors.Wrap(err, "faield to convert col to field")
		}
		fs = append(fs, f)
	}
	s.Fields = fs
	return s, nil
}

// PgExecuteStructTmpl execute struct template with *Struct
func PgExecuteStructTmpl(st *Struct) ([]byte, error) {
	var src []byte
	tpl, err := template.New("struct").Parse(structTmpl)
	if err != nil {
		return src, errors.Wrap(err, "failed to parse template")
	}
	buf := new(bytes.Buffer)
	if err := tpl.Execute(buf, st); err != nil {
		return src, errors.Wrap(err, "failed to execute template")
	}
	src, err = format.Source(buf.Bytes())
	if err != nil {
		return src, errors.Wrap(err, "failed to format source code")
	}
	return src, nil
}

// PgCreateStruct creates struct from given schema
func PgCreateStruct(db Queryer, schema, typeMapPath string) ([]byte, error) {
	var src []byte
	tbls, err := PgLoadTableDef(db, schema)
	if err != nil {
		return src, errors.Wrap(err, "faield to load table definitions")
	}
	cfg := &PgTypeMapConfig{}
	if typeMapPath == "" {
		if _, err := toml.Decode(typeMap, cfg); err != nil {
			return src, errors.Wrap(err, "faield to read type map")
		}
	} else {
		if _, err := toml.DecodeFile(typeMapPath, cfg); err != nil {
			return src, errors.Wrap(err, fmt.Sprintf("failed to decode type map file %s", typeMapPath))
		}
	}
	for _, tbl := range tbls {
		st, err := PgTableToStruct(tbl, cfg)
		if err != nil {
			return src, errors.Wrap(err, "faield to convert table definition to struct")
		}
		s, err := PgExecuteStructTmpl(st)
		if err != nil {
			return src, errors.Wrap(err, "faield to execute template")
		}
		src = append(src, s...)
	}
	return src, nil
}
