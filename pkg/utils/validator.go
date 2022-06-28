package util

import (
	"fmt"

	div2alpha1 "opendilab.org/di-orchestrator/pkg/api/v2alpha1"
)

type Validator func(job *div2alpha1.DIJob) error
type Validators []Validator

func (f Validators) Apply(job *div2alpha1.DIJob) error {
	for _, filter := range f {
		if err := filter(job); err != nil {
			return err
		}
	}
	return nil
}

var (
	TaskTypeNameValidator = func(job *div2alpha1.DIJob) error {
		taskTypeNumber := map[div2alpha1.TaskType]int{} //record taskType and its number, (learner, collector, evaluator)'s number must be one
		taskNames := map[string]int{}                   //record taskName and its number, (learner, collector, evaluator, none)'s name must be unique(number is one).
		for _, task := range job.Spec.Tasks {
			taskTypeNumber[task.Type]++
			if taskTypeNumber[task.Type] > 1 && task.Type != div2alpha1.TaskTypeNone { // has more than one typed task
				return fmt.Errorf("the number of %s task is more than one", task.Type)
			}
			if task.Type == div2alpha1.TaskTypeNone && task.Name == "" { // none type task must has a name
				return fmt.Errorf("none type task has no name")
			}
			taskNames[task.Name]++
		}
		for name, number := range taskNames { // check every name is unique
			if number > 1 {
				return fmt.Errorf("there is more than one task named %s", name)
			}
		}
		return nil
	}
)
