package browsers

import (
	"github.com/HouzuoGuo/laitos/browserp"
	"github.com/HouzuoGuo/laitos/misc"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"
)

func TestInteractiveBrowser(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("this test involves docker daemon operation, it requires root privilege.")
	}
	// CircleCI container cannot operate docker daemon
	misc.SkipTestIfCI(t)
	renderOutput, err := ioutil.TempDir("", "laitos-TestInteractiveBrowser-browsers-render")
	if err != nil {
		t.Fatal(err)
	}
	instance := &Instance{
		RenderImageDir:     renderOutput,
		Port:               41599,
		AutoKillTimeoutSec: 300,
	}
	if err := instance.Start(); err != nil {
		t.Fatal(err)
	}
	defer instance.Kill()
	// Prepare docker operation for SlimerJS
	PrepareDocker(misc.Logger{})

	// Browse distrowatch home page
	if err := instance.GoTo(browserp.GoodUserAgent, "https://distrowatch.com/", 1024, 1024); err != nil {
		t.Fatal(err, instance.GetDebugOutput())
	}
	// Expect page to be ready soon
	time.Sleep(30 * time.Second)
	if err := instance.RenderPage(); err != nil {
		t.Fatal(err)
	}
	if stat, err := os.Stat(instance.GetRenderPageFilePath()); err != nil || stat.Size() < 4096 {
		t.Fatal(err, stat.Size(), instance.GetDebugOutput())
	}
	// Expect some output to be already present in output buffer
	t.Log(instance.GetDebugOutput())
	// The image render action should have written a line of log that looks like "POST /redraw - {}: true\n"
	if out := instance.GetDebugOutput(); !strings.Contains(out, "/redraw - {}: true") {
		t.Fatalf(out)
	}
	// Try several other browser actions
	if err := instance.GoBack(); err != nil {
		t.Fatal(err)
	}
	if err := instance.GoForward(); err != nil {
		t.Fatal(err)
	}
	if err := instance.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := instance.Pointer(browserp.PointerTypeClick, browserp.PointerButtonRight, 100, 100); err != nil {
		t.Fatal(err)
	}
	// Different from PhantomJS, rapid keyboard control input causes SlimerJS error, hence the delay.
	time.Sleep(1 * time.Second)
	if err := instance.SendKey("test string", 0); err != nil {
		t.Fatal(err)
	}
	// Different from PhantomJS, rapid keyboard control input causes SlimerJS error, hence the delay.
	time.Sleep(1 * time.Second)
	if err := instance.SendKey("", 1234); err != nil {
		t.Fatal(err)
	}
	// Repeatedly stopping instance should have no negative consequence
	instance.Kill()
	instance.Kill()
	instance.Kill()
}

func TestLineOrientedBrowser(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("this test involves docker daemon operation, it requires root privilege.")
	}
	// CircleCI container cannot operate docker daemon
	misc.SkipTestIfCI(t)
	renderOutput, err := ioutil.TempDir("", "laitos-TestInteractiveBrowser-browsers-render")
	if err != nil {
		t.Fatal(err)
	}
	instance := &Instance{
		RenderImageDir:     renderOutput,
		Port:               51600,
		AutoKillTimeoutSec: 300,
	}
	if err := instance.Start(); err != nil {
		t.Fatal(err)
	}
	defer instance.Kill()
	// Prepare docker operation for SlimerJS
	PrepareDocker(misc.Logger{})
	// Browse distrowatch home page
	if err := instance.GoTo(browserp.GoodUserAgent, "https://distrowatch.com/", 1024, 1024); err != nil {
		t.Fatal(err, instance.GetDebugOutput())
	}
	// Expect page to be ready in a few seconds
	time.Sleep(30 * time.Second)
	delay := func() {
		time.Sleep(3 * time.Second)
	}
	// Navigate to first element
	firstElements, err := instance.LONextElement()
	if err != nil || len(firstElements) != 3 {
		t.Fatal(err, firstElements)
	}
	// [0] should be empty because there is no previous element
	if firstElements[0].TagName != "" || firstElements[1].TagName == "" || firstElements[2].TagName == "" {
		t.Fatal(err, firstElements)
	}
	delay()
	// Navigate to second element
	secondElements, err := instance.LONextElement()
	if err != nil || len(secondElements) != 3 {
		t.Fatal(err, secondElements)
	}
	delay()
	// [1] should match the previous element's next element
	if secondElements[1].TagName != firstElements[2].TagName || secondElements[1].TagName == "" || secondElements[2].TagName == "" {
		t.Fatal(err, secondElements[1].TagName, firstElements[2].TagName, secondElements)
	}
	delay()
	// Navigate all the way to the bottom
	elements, err := instance.LONextNElements(10000)
	if err != nil || len(elements) < 30 {
		t.Fatal(err, elements)
	}
	delay()
	// After having reached the bottom, calling next should continue to stay at the bottom.
	lastElements, err := instance.LONextElement()
	if err != nil || lastElements[1].TagName != elements[len(elements)-1].TagName {
		t.Fatal(err, lastElements)
	}
	delay()
	// Go back to the start
	if err := instance.LOResetNavigation(); err != nil {
		t.Fatal(err)
	}
	delay()
	revisitFirstElements, err := instance.LONextElement()
	if err != nil || len(revisitFirstElements) != 3 ||
		revisitFirstElements[0].TagName != "" ||
		revisitFirstElements[1].TagName != firstElements[1].TagName ||
		revisitFirstElements[2].TagName != firstElements[2].TagName {
		t.Fatal(err, revisitFirstElements, firstElements)
	}
	delay()
	// Try pointer and value actions
	if err := instance.LOPointer(browserp.PointerTypeMove, browserp.PointerButtonLeft); err != nil {
		t.Fatal(err)
	}
	delay()
	if err := instance.LOSetValue("test value"); err != nil {
		t.Fatal(err)
	}
	delay()
	// Re-visit the second element
	revisitSecondElements, err := instance.LONextElement()
	if err != nil || len(revisitSecondElements) != 3 ||
		revisitSecondElements[0].TagName != secondElements[0].TagName ||
		revisitSecondElements[1].TagName != secondElements[1].TagName ||
		revisitSecondElements[2].TagName != secondElements[2].TagName {
		t.Fatal(err, revisitSecondElements, secondElements)
	}
	// Repeatedly stopping instance should have no negative consequence
	instance.Kill()
	instance.Kill()
	instance.Kill()
}
