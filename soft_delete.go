package soft_delete

import (
	"database/sql/driver"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
	"reflect"
)

type DeletedAt bool

var (
	FlagDeleted = true
	FlagActived = false
)

func (DeletedAt) QueryClauses(f *schema.Field) []clause.Interface {
	return []clause.Interface{SoftDeleteQueryClause{Field: f}}
}

// 实现 driver.Valuer 接口，将 BoolType 转换为数据库中的值
func (b DeletedAt) Value() (driver.Value, error) {
	if b {
		return true, nil
	}
	return false, nil
}

// 实现 sql.Scanner 接口，从数据库中的值将其转换为 BoolType
func (b *DeletedAt) Scan(value interface{}) error {
	boolVal, ok := value.(bool)
	if !ok {
		return errors.New("Invalid data type for DeletedAt")
	}

	if boolVal {
		*b = true
	} else {
		*b = false
	}

	return nil
}

type SoftDeleteQueryClause struct {
	Field *schema.Field
}

func (sd SoftDeleteQueryClause) Name() string {
	return ""
}

func (sd SoftDeleteQueryClause) Build(clause.Builder) {
}

func (sd SoftDeleteQueryClause) MergeClause(*clause.Clause) {
}

func (sd SoftDeleteQueryClause) ModifyStatement(stmt *gorm.Statement) {
	if _, ok := stmt.Clauses["soft_delete_enabled"]; !ok && !stmt.Statement.Unscoped {
		if c, ok := stmt.Clauses["WHERE"]; ok {
			if where, ok := c.Expression.(clause.Where); ok && len(where.Exprs) >= 1 {
				for _, expr := range where.Exprs {
					if orCond, ok := expr.(clause.OrConditions); ok && len(orCond.Exprs) == 1 {
						where.Exprs = []clause.Expression{clause.And(where.Exprs...)}
						c.Expression = where
						stmt.Clauses["WHERE"] = c
						break
					}
				}
			}
		}

		if sd.Field.DefaultValue == "null" {
			stmt.AddClause(clause.Where{Exprs: []clause.Expression{
				clause.Eq{Column: clause.Column{Table: clause.CurrentTable, Name: sd.Field.DBName}, Value: nil},
			}})
		} else {
			stmt.AddClause(clause.Where{Exprs: []clause.Expression{
				clause.Eq{Column: clause.Column{Table: clause.CurrentTable, Name: sd.Field.DBName}, Value: FlagActived},
			}})
		}
		stmt.Clauses["soft_delete_enabled"] = clause.Clause{}
	}
}

func (DeletedAt) DeleteClauses(f *schema.Field) []clause.Interface {
	softDeleteClause := SoftDeleteDeleteClause{
		Field:    f,
		DataType: getTimeType(),
	}
	return []clause.Interface{softDeleteClause}
}

func (DeletedAt) UpdateClauses(f *schema.Field) []clause.Interface {
	return []clause.Interface{SoftDeleteUpdateClause{Field: f}}
}

type SoftDeleteUpdateClause struct {
	Field *schema.Field
}

func (sd SoftDeleteUpdateClause) Name() string {
	return ""
}

func (sd SoftDeleteUpdateClause) Build(clause.Builder) {
}

func (sd SoftDeleteUpdateClause) MergeClause(*clause.Clause) {
}

func (sd SoftDeleteUpdateClause) ModifyStatement(stmt *gorm.Statement) {
	if stmt.SQL.Len() == 0 && !stmt.Statement.Unscoped {
		SoftDeleteQueryClause(sd).ModifyStatement(stmt)
	}
}

type SoftDeleteDeleteClause struct {
	Field         *schema.Field
	Flag          bool
	DataType      schema.DataType
	DeleteAtField *schema.Field
}

func (sd SoftDeleteDeleteClause) Name() string {
	return ""
}

func (sd SoftDeleteDeleteClause) Build(clause.Builder) {
}

func (sd SoftDeleteDeleteClause) MergeClause(*clause.Clause) {
}

func (sd SoftDeleteDeleteClause) ModifyStatement(stmt *gorm.Statement) {
	if stmt.SQL.Len() == 0 && !stmt.Statement.Unscoped {
		var (
			set clause.Set
		)

		if deleteAtField := sd.DeleteAtField; deleteAtField != nil {
			var value interface{}
			if deleteAtField.GORMDataType == "bool" {
				value = true
			}
			set = append(set, clause.Assignment{Column: clause.Column{Name: deleteAtField.DBName}, Value: value})
			stmt.SetColumn(deleteAtField.DBName, value, true)
		}

		set = append(clause.Set{{Column: clause.Column{Name: sd.Field.DBName}, Value: FlagDeleted}}, set...)
		stmt.SetColumn(sd.Field.DBName, FlagDeleted, true)
		stmt.AddClause(set)

		if stmt.Schema != nil {
			_, queryValues := schema.GetIdentityFieldValuesMap(stmt.Context, stmt.ReflectValue, stmt.Schema.PrimaryFields)
			column, values := schema.ToQueryValues(stmt.Table, stmt.Schema.PrimaryFieldDBNames, queryValues)

			if len(values) > 0 {
				stmt.AddClause(clause.Where{Exprs: []clause.Expression{clause.IN{Column: column, Values: values}}})
			}

			if stmt.ReflectValue.CanAddr() && stmt.Dest != stmt.Model && stmt.Model != nil {
				_, queryValues = schema.GetIdentityFieldValuesMap(stmt.Context, reflect.ValueOf(stmt.Model), stmt.Schema.PrimaryFields)
				column, values = schema.ToQueryValues(stmt.Table, stmt.Schema.PrimaryFieldDBNames, queryValues)

				if len(values) > 0 {
					stmt.AddClause(clause.Where{Exprs: []clause.Expression{clause.IN{Column: column, Values: values}}})
				}
			}
		}

		SoftDeleteQueryClause{Field: sd.Field}.ModifyStatement(stmt)
		stmt.AddClauseIfNotExists(clause.Update{})
		stmt.Build(stmt.DB.Callback().Update().Clauses...)
	}
}

func getTimeType() schema.DataType {
	return schema.Bool
}
