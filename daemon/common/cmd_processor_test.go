package common

import (
	"github.com/HouzuoGuo/laitos/misc"
	"github.com/HouzuoGuo/laitos/toolbox"
	"github.com/HouzuoGuo/laitos/toolbox/filter"
	"reflect"
	"testing"
)

func TestCommandProcessorProcess(t *testing.T) {
	// Prepare feature set - the shell execution feature should be available even without configuration
	features := &toolbox.FeatureSet{}
	if err := features.Initialise(); err != nil {
		t.Fatal(features)
	}
	// Prepare all kinds of command bridges
	commandBridges := []filter.CommandFilter{
		&filter.PINAndShortcuts{PIN: "mypin"},
		&filter.TranslateSequences{Sequences: [][]string{{"alpha", "beta"}}},
	}
	// Prepare all kinds of result bridges
	resultBridges := []filter.ResultFilter{
		&filter.ResetCombinedText{},
		&filter.LintText{TrimSpaces: true, MaxLength: 2},
		&filter.NotifyViaEmail{},
	}

	proc := CommandProcessor{
		Features:       features,
		CommandFilters: commandBridges,
		ResultFilters:  resultBridges,
	}

	// Try mismatching PIN so that command bridge return early
	cmd := toolbox.Command{TimeoutSec: 5, Content: "badpin.secho alpha"}
	result := proc.Process(cmd)
	if !reflect.DeepEqual(result.Command, cmd) ||
		result.Error != filter.ErrPINAndShortcutNotFound || result.Output != "" ||
		result.CombinedOutput != filter.ErrPINAndShortcutNotFound.Error()[0:2] {
		t.Fatalf("%+v", result)
	}

	// Run a failing command
	cmd = toolbox.Command{TimeoutSec: 5, Content: "mypin.secho alpha && false"}
	result = proc.Process(cmd)
	if !reflect.DeepEqual(result.Command, toolbox.Command{TimeoutSec: 5, Content: ".secho beta && false"}) ||
		result.Error == nil || result.Output != "beta\n" || result.CombinedOutput != result.Error.Error()[0:2] {
		t.Fatalf("%+v", result)
	}

	// Run a command that does not trigger a configured feature
	cmd = toolbox.Command{TimeoutSec: 5, Content: "mypin.tg"}
	result = proc.Process(cmd)
	if !reflect.DeepEqual(result.Command, toolbox.Command{TimeoutSec: 5, Content: ".tg"}) ||
		result.Error != ErrBadPrefix || result.Output != "" || result.CombinedOutput != ErrBadPrefix.Error()[0:2] {
		t.Fatalf("%+v", result)
	}

	// Run a successful command
	cmd = toolbox.Command{TimeoutSec: 5, Content: "mypin.secho alpha"}
	result = proc.Process(cmd)
	if !reflect.DeepEqual(result.Command, toolbox.Command{TimeoutSec: 5, Content: ".secho beta"}) ||
		result.Error != nil || result.Output != "beta\n" || result.CombinedOutput != "be" {
		t.Fatalf("%+v", result)
	}
	// Test the tolerance to extra spaces in feature prefix matcher
	cmd = toolbox.Command{TimeoutSec: 5, Content: " mypin .s echo alpha "}
	result = proc.Process(cmd)
	if !reflect.DeepEqual(result.Command, toolbox.Command{TimeoutSec: 5, Content: ".s echo beta"}) ||
		result.Error != nil || result.Output != "beta\n" || result.CombinedOutput != "be" {
		t.Fatalf("%+v", result)
	}

	// Override PLT but PLT parameter values are not given
	cmd = toolbox.Command{TimeoutSec: 5, Content: "mypin  .plt   sadf asdf "}
	result = proc.Process(cmd)
	if !reflect.DeepEqual(result.Command, toolbox.Command{TimeoutSec: 5, Content: ".plt   sadf asdf"}) ||
		result.Error != ErrBadPLT || result.Output != "" || result.CombinedOutput != ErrBadPLT.Error()[0:2] {
		t.Fatalf("'%v' '%v' '%v' '%v'", result.Error, result.Output, result.CombinedOutput, result.Command)
	}
	// Override PLT using good PLT parameter values
	cmd = toolbox.Command{TimeoutSec: 1, Content: "mypin  .plt  2, 5. 3  .s  sleep 2 && echo -n 0123456789 "}
	result = proc.Process(cmd)
	if !reflect.DeepEqual(result.Command, toolbox.Command{TimeoutSec: 3, Content: ".plt  2, 5. 3  .s  sleep 2 && echo -n 0123456789"}) ||
		result.Error != nil || result.Output != "0123456789" || result.CombinedOutput != "23456" {
		t.Fatalf("'%v' '%v' '%v' '%+v'", result.Error, result.Output, result.CombinedOutput, result.Command)
	}

	// Trigger emergency lock down and try
	misc.TriggerEmergencyLockDown()
	cmd = toolbox.Command{TimeoutSec: 1, Content: "mypin  .plt  2, 5. 3  .s  sleep 2 && echo -n 0123456789 "}
	if result := proc.Process(cmd); result.Error != misc.ErrEmergencyLockDown {
		t.Fatal(result)
	}
	misc.EmergencyLockDown = false
}

func TestCommandProcessorIsSaneForInternet(t *testing.T) {
	proc := CommandProcessor{
		Features:       nil,
		CommandFilters: nil,
		ResultFilters:  nil,
	}
	if !proc.IsEmpty() {
		t.Fatal("not empty")
	}
	if errs := proc.IsSaneForInternet(); len(errs) != 3 {
		t.Fatal(errs)
	}
	// FeatureSet is assigned but not initialised
	proc.Features = &toolbox.FeatureSet{}
	if errs := proc.IsSaneForInternet(); len(errs) != 3 {
		t.Fatal(errs)
	}
	// Good feature set
	if err := proc.Features.Initialise(); err != nil {
		t.Fatal(err)
	}
	if errs := proc.IsSaneForInternet(); len(errs) != 2 {
		t.Fatal(errs)
	}
	// No PIN bridge
	proc.CommandFilters = []filter.CommandFilter{}
	if errs := proc.IsSaneForInternet(); len(errs) != 2 {
		t.Fatal(errs)
	}
	// PIN bridge has short PIN
	proc.CommandFilters = []filter.CommandFilter{&filter.PINAndShortcuts{PIN: "aaaaaa"}}
	if errs := proc.IsSaneForInternet(); len(errs) != 2 {
		t.Fatal(errs)
	}
	// Despite PIN being very short, the command processor is not without configuration.
	if proc.IsEmpty() {
		t.Fatal("should not be empty")
	}
	// PIN bridge has nothing
	proc.CommandFilters = []filter.CommandFilter{&filter.PINAndShortcuts{}}
	if errs := proc.IsSaneForInternet(); len(errs) != 2 {
		t.Fatal(errs)
	}
	// Good PIN bridge
	proc.CommandFilters = []filter.CommandFilter{&filter.PINAndShortcuts{PIN: "very-long-pin"}}
	if errs := proc.IsSaneForInternet(); len(errs) != 1 {
		t.Fatal(errs)
	}
	// No linter bridge
	proc.ResultFilters = []filter.ResultFilter{}
	if errs := proc.IsSaneForInternet(); len(errs) != 1 {
		t.Fatal(errs)
	}
	// Linter bridge has out-of-range max length
	proc.ResultFilters = []filter.ResultFilter{&filter.LintText{MaxLength: 1}}
	if errs := proc.IsSaneForInternet(); len(errs) != 1 {
		t.Fatal(errs)
	}
	// Good linter bridge
	proc.ResultFilters = []filter.ResultFilter{&filter.LintText{MaxLength: 35}}
	if errs := proc.IsSaneForInternet(); len(errs) != 0 {
		t.Fatal(errs)
	}

}

func TestGetTestCommandProcessor(t *testing.T) {
	if proc := GetTestCommandProcessor(); proc == nil {
		t.Fatal("did not return")
	} else if testErrs := proc.Features.SelfTest(); len(testErrs) != 0 {
		t.Fatal(testErrs)
	} else if saneErrs := proc.IsSaneForInternet(); len(saneErrs) > 0 {
		t.Fatal(saneErrs)
	} else if result := proc.Process(toolbox.Command{Content: "verysecret .elog", TimeoutSec: 10}); result.Error != nil {
		t.Fatal(result.Error)
	}
}

func TestGetEmptyCommandProcessor(t *testing.T) {
	if proc := GetEmptyCommandProcessor(); proc == nil {
		t.Fatal("did not return")
	} else if testErrs := proc.Features.SelfTest(); len(testErrs) != 0 {
		t.Fatal(testErrs)
	} else if saneErrs := proc.IsSaneForInternet(); len(saneErrs) > 0 {
		t.Fatal(saneErrs)
	} else if result := proc.Process(toolbox.Command{Content: "verysecret .elog", TimeoutSec: 10}); result.Error == nil {
		t.Fatal("did not error")
	}
}
