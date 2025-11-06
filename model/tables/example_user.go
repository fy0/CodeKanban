package tables

import "go-template/utils/model_base"

// ExampleUserTable demonstrates how to declare and register a simple GORM model.
type ExampleUserTable struct {
	model_base.StringPKBaseModel
	Name string `gorm:"size:128" json:"name"`
	Note string `gorm:"size:255" json:"note"`
}

func (*ExampleUserTable) TableName() string {
	return "example_users"
}
