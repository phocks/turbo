package core

import (
	"fmt"
	"regexp"
	"testing"

	testifyAssert "github.com/stretchr/testify/assert"
	"github.com/vercel/turbo/cli/internal/fs"
	"github.com/vercel/turbo/cli/internal/graph"
	"gotest.tools/v3/assert"

	"github.com/pyr-sh/dag"
)

var _workspaceGraphDefinition = map[string][]string{
	"workspace-a": {"workspace-c"}, // a depends on c
	"workspace-b": {"workspace-c"}, // b depends on c
	"workspace-c": {},
}

func TestPrepare_PersistentDependencies_Topological(t *testing.T) {
	completeGraph, workspaces := _buildCompleteGraph(_workspaceGraphDefinition)

	devTask := fs.TaskDefinition{Persistent: true, TopologicalDependencies: []string{"dev"}}
	completeGraph.Pipeline = fs.Pipeline{"dev": devTask}

	engine := NewEngine(completeGraph)

	// Make this Task Graph:
	// dev
	// └── ^dev
	//
	// With this workspace graph, that means:
	//
	// workspace-a#dev
	// └── workspace-c#dev
	// workspace-b#dev
	// └── workspace-c#dev

	// "dev": dependsOn: ["^dev"] (where dev is persistent)
	engine.AddTask(&Task{Name: "dev", TaskDefinition: devTask})

	opts := &EngineBuildingOptions{
		Packages:  workspaces,
		TaskNames: []string{"dev"},
	}

	err := engine.Prepare(opts)
	assert.NilError(t, err, "Failed to prepare engine")

	// do the validation
	actualErr := engine.ValidatePersistentDependencies(completeGraph)

	// Use a regex here, because depending on the order the graph is walked,
	// either workspace-a or workspace-b could throw the error first.
	expected := regexp.MustCompile("\"workspace-c#dev\" is a persistent task, \"workspace-[a|b]#dev\" cannot depend on it")
	testifyAssert.Regexp(t, expected, actualErr)
}

func TestPrepare_PersistentDependencies_SameWorkspace(t *testing.T) {
	completeGraph, workspaces := _buildCompleteGraph(_workspaceGraphDefinition)

	buildTask := fs.TaskDefinition{Persistent: false, TaskDependencies: []string{"dev"}}
	devTask := fs.TaskDefinition{Persistent: true}

	completeGraph.Pipeline = fs.Pipeline{
		"build": buildTask,
		"dev":   devTask,
	}

	engine := NewEngine(completeGraph)

	// Make this Task Graph:
	// build
	// └── dev
	//
	// With this workspace graph, that means:
	//
	// workspace-a#build
	// └── workspace-a#dev
	// workspace-b#build
	// └── workspace-b#dev
	// workspace-c#build
	// └── workspace-c#dev

	// "build": dependsOn: ["dev"] (where build is not, but "dev" is persistent)
	engine.AddTask(&Task{Name: "build", TaskDefinition: buildTask})
	engine.AddTask(&Task{Name: "dev", TaskDefinition: devTask})

	opts := &EngineBuildingOptions{
		Packages:  workspaces,
		TaskNames: []string{"build"},
	}

	err := engine.Prepare(opts)
	assert.NilError(t, err, "Failed to prepare engine")

	// do the validation
	actualErr := engine.ValidatePersistentDependencies(completeGraph)

	// Note: this regex is not perfect, becase it doesn't validate that the a|b|c is the same in both positions,
	// but that's ok. It is unlikely that the error message will be wrong here. (And even if it is,
	// the feature that is being tested would still be working)
	expected := regexp.MustCompile("\"workspace-[a|b|c]#dev\" is a persistent task, \"workspace-[a|b|c]#build\" cannot depend on it")
	testifyAssert.Regexp(t, expected, actualErr)
}

func TestPrepare_PersistentDependencies_WorkspaceSpecific(t *testing.T) {
	completeGraph, workspaces := _buildCompleteGraph(_workspaceGraphDefinition)
	buildTask := fs.TaskDefinition{Persistent: false, TaskDependencies: []string{"workspace-b#dev"}}
	bDevTask := fs.TaskDefinition{Persistent: true}
	completeGraph.Pipeline = fs.Pipeline{
		"build":           buildTask,
		"workspace-b#dev": bDevTask,
	}

	engine := NewEngine(completeGraph)

	// Make this Task Graph:
	// build
	// └── workspace-b#dev
	//
	// With this workspace graph, that means:
	//
	// workspace-a#build
	// └── workspace-b#dev
	// workspace-b#build
	// └── workspace-b#dev
	// workspace-c#build
	// └── workspace-b#dev

	// "build": dependsOn: ["workspace-b#dev"]
	engine.AddTask(&Task{Name: "build", TaskDefinition: buildTask})

	// workspace-b#dev is persistent, and has no dependencies
	engine.AddTask(&Task{Name: "workspace-b#dev", TaskDefinition: bDevTask})

	opts := &EngineBuildingOptions{
		Packages:  workspaces,
		TaskNames: []string{"build"},
	}

	err := engine.Prepare(opts)
	assert.NilError(t, err, "Failed to prepare engine")

	// do the validation
	actualErr := engine.ValidatePersistentDependencies(completeGraph)

	// Depending on the order the graph is walked in, workspace a, b, or c, could throw the error first
	// but the persistent task is consistently workspace-b.
	expected := regexp.MustCompile("\"workspace-b#dev\" is a persistent task, \"workspace-[a|b|c]#build\" cannot depend on it")
	testifyAssert.Regexp(t, expected, actualErr, "")
}

func TestPrepare_PersistentDependencies_CrossWorkspace(t *testing.T) {
	completeGraph, workspaces := _buildCompleteGraph(_workspaceGraphDefinition)
	aDevTask := fs.TaskDefinition{Persistent: true, TaskDependencies: []string{"workspace-b#dev"}}
	bDevTask := fs.TaskDefinition{Persistent: true}
	completeGraph.Pipeline = fs.Pipeline{
		"workspace-a#dev": aDevTask,
		"workspace-b#dev": bDevTask,
	}
	engine := NewEngine(completeGraph)

	// Make this Task Graph:
	// workspace-a#dev
	// └── workspace-b#dev

	// workspace-a#dev specifically dependsOn workspace-b#dev
	// Note: AddDep() is necessary in addition to AddTask() to set up this dependency
	err := engine.AddDep("workspace-b#dev", "workspace-a#dev")
	assert.NilError(t, err, "Failed to prepare engine")

	engine.AddTask(&Task{Name: "workspace-a#dev", TaskDefinition: aDevTask})

	// workspace-b#dev dependsOn nothing else
	engine.AddTask(&Task{Name: "workspace-b#dev", TaskDefinition: bDevTask})

	opts := &EngineBuildingOptions{
		Packages:  workspaces,
		TaskNames: []string{"dev"},
	}

	prepErr := engine.Prepare(opts)
	assert.NilError(t, prepErr, "Failed to prepare engine")

	// do the validation
	actualErr := engine.ValidatePersistentDependencies(completeGraph)
	testifyAssert.EqualError(t, actualErr, "\"workspace-b#dev\" is a persistent task, \"workspace-a#dev\" cannot depend on it")
}

func TestPrepare_PersistentDependencies_RootWorkspace(t *testing.T) {
	completeGraph, workspaces := _buildCompleteGraph(_workspaceGraphDefinition)
	// Add in a "dev" task into the root workspace, so it exists
	completeGraph.WorkspaceInfos["//"].Scripts["dev"] = "echo \"root dev task\""

	buildTask := fs.TaskDefinition{TaskDependencies: []string{"//#dev"}}
	rootDevTask := fs.TaskDefinition{Persistent: true}
	completeGraph.Pipeline = fs.Pipeline{
		"build":  buildTask,
		"//#dev": rootDevTask,
	}
	engine := NewEngine(completeGraph)

	// Make this Task Graph:
	// build
	// └── //#dev
	//
	// With this workspace graph, that means:
	//
	// workspace-a#build
	// └── //#dev
	// workspace-b#build
	// └── //#dev
	// workspace-c#build
	// └── //#dev

	// build task depends on the root dev task
	engine.AddTask(&Task{Name: "build", TaskDefinition: buildTask})

	// Add the persistent task in the root workspace
	engine.AddTask(&Task{Name: "//#dev", TaskDefinition: rootDevTask})

	// prepare the engine
	opts := &EngineBuildingOptions{
		Packages:  workspaces,
		TaskNames: []string{"build"},
	}

	err := engine.Prepare(opts)
	assert.NilError(t, err, "Failed to prepare engine")

	actualErr := engine.ValidatePersistentDependencies(completeGraph)
	// Use a regex here, because depending on the order the graph is walked,
	// workspace-a, b or c could throw the error first.
	expected := regexp.MustCompile("\"//#dev\" is a persistent task, \"workspace-[a|b|c]#build\" cannot depend on it")

	testifyAssert.Regexp(t, expected, actualErr)
}

func TestPrepare_PersistentDependencies_Unimplemented(t *testing.T) {
	completeGraph, workspaces := _buildCompleteGraph(_workspaceGraphDefinition)
	devTask := fs.TaskDefinition{Persistent: true, TopologicalDependencies: []string{"dev"}}
	completeGraph.Pipeline = fs.Pipeline{
		"dev": devTask,
	}
	engine := NewEngine(completeGraph)

	// Make this Task Graph:
	// dev
	// └── ^dev
	//
	// With this workspace graph, that means:
	//
	// workspace-a#dev
	// └── workspace-c#dev (but this isn't implemented)
	// workspace-b#dev
	// └── workspace-c#dev (but this isn't implemented)

	// Remove "dev" script from workspace-c. workspace-a|b will still implement,
	// but since no topological dependencies implement, this test can ensure there is no error
	delete(completeGraph.WorkspaceInfos["workspace-c"].Scripts, "dev")

	// "dev": dependsOn: ["^dev"] (dev is persistent, but workspace-c does not implement dev)
	engine.AddTask(&Task{Name: "dev", TaskDefinition: devTask})

	opts := &EngineBuildingOptions{
		Packages:  workspaces,
		TaskNames: []string{"dev"},
	}

	err := engine.Prepare(opts)
	assert.NilError(t, err, "Failed to prepare engine")

	// do the validation
	actualErr := engine.ValidatePersistentDependencies(completeGraph)

	testifyAssert.Nil(t, actualErr)
}

func TestPrepare_PersistentDependencies_Topological_SkipDepImplementedTask(t *testing.T) {
	var workspaceGraphDefinition = map[string][]string{
		"workspace-a": {"workspace-b"}, // a depends on b
		"workspace-b": {"workspace-c"}, // b depends on c
		"workspace-c": {},
	}
	devTask := fs.TaskDefinition{Persistent: true, TopologicalDependencies: []string{"dev"}}
	completeGraph, workspaces := _buildCompleteGraph(workspaceGraphDefinition)
	completeGraph.Pipeline = fs.Pipeline{"dev": devTask}

	// Make this Task Graph:
	// dev
	// └── ^dev
	//
	// With this workspace graph, that means:
	//
	// workspace-a#dev
	// └── workspace-b#dev (but this isn't implemented)
	// 		 └── workspace-c#dev

	// remove b's dev script, so there's a skip in the middle
	delete(completeGraph.WorkspaceInfos["workspace-b"].Scripts, "dev")

	engine := NewEngine(completeGraph)

	// "dev": dependsOn: ["^dev"] (where dev is persistent)
	engine.AddTask(&Task{
		Name:           "dev",
		TaskDefinition: devTask,
	})

	opts := &EngineBuildingOptions{
		Packages:  workspaces,
		TaskNames: []string{"dev"},
	}

	err := engine.Prepare(opts)
	assert.NilError(t, err, "Failed to prepare engine")

	// do the validation
	actualErr := engine.ValidatePersistentDependencies(completeGraph)

	// Note: This error is interesting because workspace-b doesn't implement dev, so saying that workspace-c
	// shouldn't depend on it. This is partly unavoidable, but partly debatable about what the error message
	// should say. Leaving as-is so we don't have to implement special casing logic to handle this case.
	testifyAssert.EqualError(t, actualErr, "\"workspace-c#dev\" is a persistent task, \"workspace-b#dev\" cannot depend on it")
}

func TestPrepare_PersistentDependencies_Topological_WithALittleExtra(t *testing.T) {
	var workspaceGraphDefinition = map[string][]string{
		"workspace-a": {"workspace-b"}, // a depends on b
		"workspace-b": {"workspace-c"}, // b depends on c
		"workspace-c": {},              // no dependencies
		"workspace-z": {},              // no dependencies, nothing depends on it, just floatin'
	}

	completeGraph, workspaces := _buildCompleteGraph(workspaceGraphDefinition)

	buildTask := fs.TaskDefinition{TopologicalDependencies: []string{"build"}}
	cBuildTask := fs.TaskDefinition{TaskDependencies: []string{"workspace-z#dev"}}
	zDevTask := fs.TaskDefinition{Persistent: true}
	completeGraph.Pipeline = fs.Pipeline{
		"build":             buildTask,
		"workspace-c#build": cBuildTask,
		"workspace-z#dev":   zDevTask,
	}

	engine := NewEngine(completeGraph)

	// Make this Task Graph:
	// build
	// └── ^build
	// workspace-c#build
	// └── workspace-z#dev
	//
	// With this workspace graph, that means:
	//
	// workspace-a#build
	// └── workspace-b#build
	// 		 └── workspace-c#build
	// 		 		 └── workspace-z#dev	// this one is persistent

	// "build": dependsOn: ["^build"]
	engine.AddTask(&Task{Name: "build", TaskDefinition: buildTask})

	// workspace-c#build also depends on workspace-z#dev
	// Note: AddDep() is necessary in addition to AddTask() to set up this dependency
	err := engine.AddDep("workspace-z#dev", "workspace-c#build")
	assert.NilError(t, err, "Failed to prepare engine")
	engine.AddTask(&Task{Name: "workspace-c#build", TaskDefinition: cBuildTask})

	// workspace-z#dev is persistent (blanket "dev" is not added, we don't need it for this test case)
	engine.AddTask(&Task{Name: "workspace-z#dev", TaskDefinition: zDevTask})

	opts := &EngineBuildingOptions{
		Packages:  workspaces,
		TaskNames: []string{"build"},
	}

	prepErr := engine.Prepare(opts)
	assert.NilError(t, prepErr, "Failed to prepare engine")

	// do the validation
	actualErr := engine.ValidatePersistentDependencies(completeGraph)
	testifyAssert.EqualError(t, actualErr, "\"workspace-z#dev\" is a persistent task, \"workspace-c#build\" cannot depend on it")
}

func TestPrepare_PersistentDependencies_CrossWorkspace_DownstreamPersistent(t *testing.T) {
	var workspaceGraphDefinition = map[string][]string{
		"workspace-a": {}, // no dependencies
		"workspace-b": {}, // no dependencies
		"workspace-c": {}, // no dependencies
		"workspace-z": {}, // no dependencies
	}
	completeGraph, workspaces := _buildCompleteGraph(workspaceGraphDefinition)
	buildTask := fs.TaskDefinition{}
	aBuildTask := fs.TaskDefinition{TaskDependencies: []string{"workspace-b#build"}}
	bBuildTask := fs.TaskDefinition{TaskDependencies: []string{"workspace-c#build"}}
	cBuildTask := fs.TaskDefinition{TaskDependencies: []string{"workspace-z#dev"}}
	zDevTask := fs.TaskDefinition{Persistent: true}
	completeGraph.Pipeline = fs.Pipeline{
		"build":             buildTask,
		"workspace-a#build": aBuildTask,
		"workspace-b#build": bBuildTask,
		"workspace-c#build": cBuildTask,
		"workspace-z#dev":   zDevTask,
	}

	engine := NewEngine(completeGraph)

	// Make this Task Graph:
	//
	// workspace-a#build
	// └── workspace-b#build
	// 		 └── workspace-c#build
	// 		 		 └── workspace-z#dev // this one is persistent
	//

	// Note: AddDep() is necessary in addition to AddTask() to set up this dependency
	err1 := engine.AddDep("workspace-b#build", "workspace-a#build") // a#build dependsOn b#build
	assert.NilError(t, err1, "Failed to prepare engine")
	err2 := engine.AddDep("workspace-c#build", "workspace-b#build") // b#build dependsOn c#build
	assert.NilError(t, err2, "Failed to prepare engine")
	err3 := engine.AddDep("workspace-z#dev", "workspace-c#build") // c#build dependsOn z#dev
	assert.NilError(t, err3, "Failed to prepare engine")

	// The default build command has no deps, it just exists to have a baseline
	engine.AddTask(&Task{Name: "build"})
	engine.AddTask(&Task{Name: "workspace-a#build", TaskDefinition: aBuildTask})
	engine.AddTask(&Task{Name: "workspace-b#build", TaskDefinition: bBuildTask})
	engine.AddTask(&Task{Name: "workspace-c#build", TaskDefinition: cBuildTask})
	engine.AddTask(&Task{Name: "workspace-z#dev", TaskDefinition: zDevTask})

	opts := &EngineBuildingOptions{
		Packages:  workspaces,
		TaskNames: []string{"build"},
	}

	prepErr := engine.Prepare(opts)
	assert.NilError(t, prepErr, "Failed to prepare engine")

	// do the validation
	actualErr := engine.ValidatePersistentDependencies(completeGraph)
	testifyAssert.EqualError(t, actualErr, "\"workspace-z#dev\" is a persistent task, \"workspace-c#build\" cannot depend on it")
}

// helper function for some of the tests to set up workspace
func _buildCompleteGraph(workspaceEasyDefinition map[string][]string) (*graph.CompleteGraph, []string) {
	var workspaceGraph dag.AcyclicGraph
	var workspaces []string

	// Turn the easy definition above into a dag.AcyclicGraph
	// Also collect just the keys of the easyDefinition
	for workspace, dependsOn := range workspaceEasyDefinition {
		workspaces = append(workspaces, workspace)
		workspaceGraph.Add(workspace)
		for _, dependsOnWorkspace := range dependsOn {
			workspaceGraph.Connect(dag.BasicEdge(workspace, dependsOnWorkspace))
		}
	}

	// build Workspace Infos
	workspaceInfos := make(graph.WorkspaceInfos)

	// Add in the root workspace. Not adding any scripts in here
	// but specific tests may add it in
	workspaceInfos["//"] = &fs.PackageJSON{
		Name:    "my-test-package",
		Scripts: map[string]string{}, // empty
	}

	// Seed some scripts for each of the workspaces since all our tests
	// mostly center around these scripts.
	for _, workspace := range workspaces {
		workspaceInfos[workspace] = &fs.PackageJSON{
			Name: workspace,
			Scripts: map[string]string{
				"build": fmt.Sprintf("echo \"%s build\"", workspace),
				"dev":   fmt.Sprintf("echo \"%s dev\"", workspace),
			},
		}
	}

	// build completeGraph struct
	completeGraph := &graph.CompleteGraph{
		WorkspaceGraph:  workspaceGraph,
		WorkspaceInfos:  workspaceInfos,
		TaskDefinitions: map[string]*fs.TaskDefinition{},
	}

	return completeGraph, workspaces
}
