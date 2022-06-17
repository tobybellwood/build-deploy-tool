package cmd

import (
	"fmt"
	"testing"

	"github.com/uselagoon/build-deploy-tool/internal/lagoon"
	"github.com/uselagoon/build-deploy-tool/internal/tasklib"
)

func Test_evaluateWhenConditionsForTaskInEnvironment(t *testing.T) {
	type args struct {
		environment tasklib.TaskEnvironment
		task        lagoon.Task
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "Successful evaluation",
			args: args{
				environment: map[string]interface{}{},
				task: lagoon.Task{
					When: "true",
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "Returns non-bool int",
			args: args{
				environment: map[string]interface{}{},
				task: lagoon.Task{
					When: "5",
				},
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "Syntax error",
			args: args{
				environment: map[string]interface{}{},
				task: lagoon.Task{
					When: "7+)3==2",
				},
			},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateWhenConditionsForTaskInEnvironment(tt.args.environment, tt.args.task)
			if (err != nil) != tt.wantErr {
				t.Errorf("evaluateWhenConditionsForTaskInEnvironment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("evaluateWhenConditionsForTaskInEnvironment() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_runTasks(t *testing.T) {
	type args struct {
		taskType                               int
		taskRunner                             iterateTaskFuncType
		lYAML                                  lagoon.YAML
		lagoonConditionalEvaluationEnvironment tasklib.TaskEnvironment
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Basic test",
			args: args{
				taskType: preRolloutTasks,
				lYAML: lagoon.YAML{
					Tasks: lagoon.Tasks{
						Prerollout: []lagoon.TaskRun{
							{
								Run: lagoon.Task{
									Command: "",
									When:    "",
								},
							},
						},
						Postrollout: nil,
					},
				},
				lagoonConditionalEvaluationEnvironment: tasklib.TaskEnvironment{
					"KEY1": "KEY2",
				},
				taskRunner: func(lagoonConditionalEvaluationEnvironment tasklib.TaskEnvironment, tasks []lagoon.Task) (bool, error) {
					if _, ok := lagoonConditionalEvaluationEnvironment["KEY1"]; !ok {
						return false, fmt.Errorf("Unable to find Key 1")
					}
					return true, nil
				},
			},
			wantErr: false,
		},
		{
			name: "Condition should fail",
			args: args{
				taskType: preRolloutTasks,
				lYAML: lagoon.YAML{
					Tasks: lagoon.Tasks{
						Prerollout: []lagoon.TaskRun{
							{
								Run: lagoon.Task{
									Command: "",
									When:    "NONEXISTANT == true",
								},
							},
						},
						Postrollout: nil,
					},
				},
				lagoonConditionalEvaluationEnvironment: tasklib.TaskEnvironment{
					"KEY1": "KEY2",
				},
				taskRunner: func(lagoonConditionalEvaluationEnvironment tasklib.TaskEnvironment, tasks []lagoon.Task) (bool, error) {
					for _, task := range tasks {
						_, err := evaluateWhenConditionsForTaskInEnvironment(lagoonConditionalEvaluationEnvironment, task)
						if err != nil {
							return true, err
						}
					}
					return false, nil
				},
			},
			wantErr: true,
		},
	}

	oldNamespace := namespace
	namespace = "default"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := runTasks(tt.args.taskRunner, tt.args.lYAML.Tasks.Prerollout, tt.args.lagoonConditionalEvaluationEnvironment); (err != nil) != tt.wantErr {
				t.Errorf("runTasks() error = %v, wantErr %v", err, tt.wantErr)
			}

		})
	}
	namespace = oldNamespace
}
