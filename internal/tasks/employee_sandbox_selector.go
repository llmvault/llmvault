package tasks

import (
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/employeesandbox"
	"gorm.io/gorm"
)

func employeeRuntimeSelector(db *gorm.DB, deps employeeruntime.CompileDeps) employeesandbox.Selector {
	selector := employeesandbox.Selector{DB: db}
	if deps.Cfg != nil {
		selector.EmployeeRuntimeImage = deps.Cfg.SandboxesRuntimeBaseImage
		selector.SpecialistRuntimeImage = deps.Cfg.SandboxesRuntimeSpecialistImage
	}
	return selector
}
