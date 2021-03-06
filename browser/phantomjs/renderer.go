package phantomjs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HouzuoGuo/laitos/inet"
	"github.com/HouzuoGuo/laitos/lalog"
	"github.com/HouzuoGuo/laitos/platform"
)

const (
	// JSCodeTemplate is not identical to the version used in SlimerJS.
	JSCodeTemplate = `try {
    var browser; // the browser page instance after very first URL is visited

    // ============== ACTIONS COMMON TO INTERACTIVE AND LINE-ORIENTED INTERFACE ==========

    // Re-render page screenshot.
    var b_redraw = function () {
        if (!browser) {
            return false;
        }
        browser.render('%s', {format: 'jpeg'});
        return true;
    };

    // Set screen shot render region.
    var b_redraw_area = function (param) {
        if (!browser) {
            return false;
        }
        browser.clipRect = {
			top: parseInt(param.top),
			left: parseInt(param.left),
			width: parseInt(param.width),
			height: parseInt(param.height)
		};
		browser.scrollPosition = {
			top: parseInt(param.top),
			left: parseInt(param.left)
		};
        return true;
    };

    // Navigate back.
    var b_back = function () {
        if (!browser) {
            return false;
        }
        b_lo_reset();
        browser.goBack();
        return true;
    };

    // Navigate forward.
    var b_forward = function () {
        if (!browser) {
            return false;
        }
        b_lo_reset();
        browser.goForward();
        return true;
    };

    // Reload the current URL (refresh).
    var b_reload = function () {
        if (!browser) {
            return false;
        }
        b_lo_reset();
        browser.reload();
        return true;
    };

    // Go to a new URL.
    var b_goto = function (param) {
        if (!browser) {
            browser = require('webpage').create();
            browser.onConsoleMessage = function (msg, line_num, src_id) {
                console.log("PAGE CONSOLE: " + msg);
            };
        }
        b_lo_reset();
        browser.settings.userAgent = param.user_agent;
		param.view_width = parseInt(param.view_width);
		param.view_height = parseInt(param.view_height);
        browser.viewportSize = {
            width: param.view_width,
            height: param.view_height
        };
        browser.clipRect = {
            top: 0,
            left: 0,
            width: param.view_width,
            height: param.view_height
        };
        browser.onResourceError = function (err) {
            console.log('b_goto error: ' + err.errorString);
        };
        browser.open(param.page_url, function (result) {
            console.log('b_goto: ' + result);
        });
        return true;
    };

    // Retrieve title and URL of the current page.
    var b_info = function () {
        var ret = '';
        if (browser) {
            ret = {
                'title': browser.evaluate(function () {
                    return document.title;
                }),
                'page_url': browser.url
            };
        }
        return ret;
    };

    // Move mouse pointer to a coordinate and optionally click a button.
    var b_pointer = function (param) {
        if (!browser) {
            return false;
        }
        browser.sendEvent(param.type, param.x, param.y, param.button);
        return true;
    };

    // Type keys into the currently focused element.
    var b_type = function (param) {
        if (!browser) {
            return false;
        }
        if (param.key_string) {
            browser.sendEvent('keypress', param.key_string);
        } else {
            browser.sendEvent('keypress', parseInt(param.key_code));
        }
        return true;
    };

    // Run a web server that receives commands from HTTP clients.
    var server = require('webserver').create().listen('127.0.0.1:%d', function (req, resp) {
        resp.statusCode = 200;
        resp.headers = {
            'Cache-Control': 'no-cache, no-store, must-revalidate',
            'Content-Type': 'application/json'
        };
        var ret = null;
        if (req.url === '/redraw') {
            // curl -X POST 'localhost:12345/redraw'
            ret = b_redraw();
        } else if (req.url === '/redraw_area') {
            // curl -X POST --data 'top=0&left=0&width=400&height=400' 'localhost:12345/redraw_area'
            ret = b_redraw_area(req.post);
        } else if (req.url === '/back') {
            ret = b_back();
        } else if (req.url === '/forward') {
            ret = b_forward();
        } else if (req.url === '/reload') {
            ret = b_reload();
        } else if (req.url === '/goto') {
            // curl -X POST --data 'user_agent=TEST&view_width=1024&view_height=1024&page_url=URL' 'localhost:12345/goto'
            ret = b_goto(req.post);
        } else if (req.url === '/info') {
            // curl -X POST 'localhost:12345/info'
            ret = b_info();
        } else if (req.url === '/pointer') {
            // curl -X POST --data 'type=click&x=111&y=222&button=left' 'localhost:12345/type'
            ret = b_pointer(req.post);
        } else if (req.url === '/type') {
            // curl -X POST --data 'key_string=test123' 'localhost:12345/type'
            ret = b_type(req.post);
        } else if (req.url === '/lo_reset') {
            // curl -X POST 'localhost:12345/lo_reset'
            ret = b_lo_reset();
        } else if (req.url === '/lo_next') {
            // curl -X POST 'localhost:12345/lo_next'
            ret = b_lo_next()
        } else if (req.url === '/lo_prev') {
            // curl -X POST 'localhost:12345/lo_prev'
            ret = b_lo_prev()
        } else if (req.url === '/lo_next_n') {
            // curl -X POST --data 'n=10' 'localhost:12345/lo_next_n'
            ret = b_lo_next_n(req.post);
        } else if (req.url === '/lo_pointer') {
            // curl -X POST --data 'type=click&button=left' 'localhost:12345/lo_pointer'
            ret = b_lo_pointer(req.post);
        } else if (req.url === '/lo_set_val') {
            // curl -X POST --data 'value=ABCDEFG' 'localhost:12345/lo_set_val'
            ret = b_lo_set_val(req.post);
        }
        console.log(req.method + ' ' + req.url + ' - ' + JSON.stringify(req.post) + ': ' + JSON.stringify(ret));
        resp.write(JSON.stringify(ret));
        resp.close();
        if (req.url === '/close') {
            phantom.exit();
        }
    });

    // ===================== ONLY FOR LINE-ORIENTED INTERFACE =================

    // The very previous element and its own previous/next element that were navigated into.
    var exact_info = null, before_info = null, after_info = null;

    // Put a string into double quotes.
    var quote_str = function (str) {
        return JSON.stringify(str);
    };

    // Return a string-encoded function body that store 4 element parameters into window object.
    var elem_info_to_stmt = function (elem_info) {
        return "function(){" +
            "window.laitos_pjs_tag = " + quote_str(elem_info === null ? '' : elem_info['tag']) + ";" +
            "window.laitos_pjs_id  = " + quote_str(elem_info === null ? '' : elem_info['id']) + ";" +
            "window.laitos_pjs_name = " + quote_str(elem_info === null ? '' : elem_info['name']) + ";" +
            "window.laitos_pjs_inner = " + quote_str(elem_info === null ? '' : elem_info['inner']) + ";" +
            "window.laitos_pjs_stop_at_first = " + (elem_info === null ? 'true' : 'false') + ";" +
            "}";
    };

    // Install several functions that help line-oriented browsing into window object.
    var lo_install_func = function () {
		window.laitos_pjs_tag = null;
		window.laitos_pjs_id = null;
		window.laitos_pjs_name = null;
		window.laitos_pjs_inner = null;
		window.laitos_pjs_stop_at_first = null;

        // Look for an element, and return brief details of the element along with its previous and next element. Give the exact match the focus.
        window.laitos_pjs_find_before_after = function (tag, id, name, inner) {
            var before = null, exact = null, after = null, stop_next = false;
            laitos_pjs_walk(document.documentElement, function (elem) {
                if (!elem) {
                    return true;
                }
                var height = elem.offsetHeight,
                    width = elem.offsetWidth,
                    elem_inner = elem.innerHTML;
                // Only consider elements that are at least 9 square pixels large and content does not look exceedingly long
                if (height > 3 && width > 3 && (!elem_inner || elem_inner && elem_inner.length < 1000)) {
                    if (stop_next) {
                        after = elem;
                        return false;
                    }
                    if (elem.tagName === tag && elem.id === id && elem.name === name && elem_inner === inner || laitos_pjs_stop_at_first) {
                        exact = elem;
                        window.laitos_pjs_current_elem = elem;
                        elem.focus();
                        stop_next = true;
                    } else {
                        before = elem;
                    }
                }
                return true;
            });
            return [
                before === null ? null : laitos_pjs_elem_to_obj(before),
                exact === null ? null : laitos_pjs_elem_to_obj(exact),
                after === null ? null : laitos_pjs_elem_to_obj(after)
            ];
        };

        // Turn a DOM element into an object that describes several of its details.
        window.laitos_pjs_elem_to_obj = function (elem) {
            return {
                "tag": elem.tagName,
                "id": elem.id,
                "name": elem.name,
                "value": elem.value,
                "inner": elem.innerHTML
            };
        };

        // Walk through DOM elements.
        window.laitos_pjs_walk = function (elem, walk_fun) {
            if (!elem) {
                return true;
            }
            for (var child = elem.childNodes, t = 0; t < child.length; t++) {
                if (!laitos_pjs_walk(child[t], walk_fun)) {
                    return false;
                }
            }
            return walk_fun(elem);
        };

        // Find elements that are immediately adjacent to the one described in parameters. Give the very last one to focus.
        window.laitos_pjs_find_after = function (tag, id, name, inner, num) {
            var ret = [], matched = false;
            laitos_pjs_walk(document.documentElement, function (elem) {
                if (!elem) {
                    return true;
                }
                var height = elem.offsetHeight,
                    width = elem.offsetWidth,
                    elem_inner = elem.innerHTML;
                // Only consider elements that are at least 9 square pixels large and content does not look exceedingly long
                if (height > 3 && width > 3 && (!elem_inner || elem_inner && elem_inner.length < 8192)) {
                    if (elem.tagName === tag && elem.id === id && elem.name === name && elem_inner === inner) {
                        matched = true;
                        return true;
                    }
                    if (matched) {
                        if (ret.length < num) {
                            window.laitos_pjs_current_elem = elem;
                            elem.focus();
                            ret.push(laitos_pjs_elem_to_obj(elem));
                        } else {
                            return false;
                        }
                    }
                }
                return true;
            });
            return ret;
        };
    };

    // Reset recorded element information so that next DOM navigation will find the first element on page.
    var b_lo_reset = function () {
        before_info = null;
        exact_info = null;
        after_info = null;
    };

    // PhantomJS has a weird bug, if in page context a null value is returned to phantomJS caller, the value turns into an empty string.
    var empty_str_to_null = function (array) {
        for (var i = 0; i < array.length; i++) {
            if (array[i] === "") {
                array[i] = null;
            }
        }
        return array;
    };

    // Navigate to the next element.
    var b_lo_next = function () {
        if (!browser) {
            return false;
        }
        browser.evaluate(lo_install_func);
        if (exact_info === null) {
            console.log('b_lo_next: visit the first element');
            // Go to the first element, null parameter value will turn on laitos_pjs_stop_at_first.
            browser.evaluateJavaScript(elem_info_to_stmt(exact_info));
            // Invoke the search function using parameters stored in window object
        } else {
            if (after_info === null) {
                // If already at last element, do not go further next.
                console.log('b_lo_next: already at last element');
                browser.evaluateJavaScript(elem_info_to_stmt(exact_info));
            } else {
                // Go to the next element
                console.log('b_lo_next: visit the next element');
                browser.evaluateJavaScript(elem_info_to_stmt(after_info));

            }
        }
        var ret = empty_str_to_null(browser.evaluate(function () {
            return laitos_pjs_find_before_after(laitos_pjs_tag, laitos_pjs_id, laitos_pjs_name, laitos_pjs_inner);
        }));
        before_info = ret[0];
        exact_info = ret[1];
        after_info = ret[2];
        return ret;
    };

    // Navigate to the previous element.
    var b_lo_prev = function () {
        if (!browser) {
            return false;
        }
        browser.evaluate(lo_install_func);

        // If before_info is null, it will naturally visit the first element of the page.
        browser.evaluateJavaScript(elem_info_to_stmt(before_info));
        var ret = empty_str_to_null(browser.evaluate(function () {
            return laitos_pjs_find_before_after(laitos_pjs_tag, laitos_pjs_id, laitos_pjs_name, laitos_pjs_inner);
        }));

        before_info = ret[0];
        exact_info = ret[1];
        after_info = ret[2];
        return ret;
    };

    // Jump forward for a designated number of elements, and return information of elements seen on the way.
    var b_lo_next_n = function (param) {
        if (!browser) {
            return false;
        }
        browser.evaluate(lo_install_func);
        // If no element has ever been navigated into, go to the first element.
        if (exact_info === null) {
            b_lo_next();
        }
        browser.evaluateJavaScript(elem_info_to_stmt(exact_info));
        browser.evaluateJavaScript("function(){window.laitos_pjs_next_n=" + param.n + ";}");

        var ret = empty_str_to_null(browser.evaluate(function () {
            return laitos_pjs_find_after(laitos_pjs_tag, laitos_pjs_id, laitos_pjs_name, laitos_pjs_inner, laitos_pjs_next_n);
        }));

        if (ret.length > 0) {
            before_info = exact_info;
            // Intentionally set both exact and after element information to that belonging to very last element
            exact_info = ret[ret.length - 1];
            after_info = ret[ret.length - 1];
        }
        return ret;
    };

    // Send a pointer event (left/right click) to the page.
    var b_lo_pointer = function (param) {
        if (!browser) {
            return false;
        }
        var x = browser.evaluate(function () {
            if (!laitos_pjs_current_elem) {
                return false;
            }
            return laitos_pjs_current_elem.getBoundingClientRect().left + window.scrollX;
        });
        var y = browser.evaluate(function () {
            if (!laitos_pjs_current_elem) {
                return false;
            }
            return laitos_pjs_current_elem.getBoundingClientRect().top + window.scrollY;
        });
        if (x === false || y === false) {
            return false;
        }
        // Instead of pointing exactly on its boarder, point a bit into the element.
        return b_pointer({
            'type': param['type'],
            'x': x + 1,
            'y': y + 1,
            'button': param['button']
        });
    };

    // Set a value to the currently focused element.
    var b_lo_set_val = function (param) {
        if (!browser) {
            return false;
        }
        browser.evaluate(lo_install_func);
        browser.evaluateJavaScript("function(){window.laitos_pjs_set_value_to=" + JSON.stringify(param.value) + ";}");

        // Give the currently focused element a new value.
        return browser.evaluate(function () {
            if (!laitos_pjs_current_elem) {
                return false;
            }
            return laitos_pjs_current_elem.getBoundingClientRect().left + window.scrollX;
        });
    };

} catch
    (err) {
    var msg = "\nPhantomJS Javascript Exception";
    msg += "\nError: " + err.toString();
    for (var p in err) {
        msg += "\n" + p.toUpperCase() + ": " + ex[p];
    }
    console.log(msg);
}`  // Template javascript code that runs on headless browser server

	// GoodUserAgent is the recommended user agent string for rendering all pages
	GoodUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:58.0) Gecko/20100101 Firefox/58.0"
)

var TagCounter = int64(0) // Increment only counter that assigns each started browser its tag. Value 0 is an invalid tag.

// Instance is a single headless browser server (PhantomJS) that acts on on commands received via HTTP.
type Instance struct {
	PhantomJSExecPath  string // Absolute or relative path to PhantomJS executable
	RenderImagePath    string // Place to store rendered web page image
	Port               int    // Port number for headless server to listen for commands on
	AutoKillTimeoutSec int    // Process is unconditionally killed after the time elapses
	Tag                string // Uniquely identifies this browser server after it is started
	Index              int    // index is the instance number assigned by renderer lifecycle management.

	serverJSFile  *os.File             // serverJSFile stores javascript code for web driver
	jsDebugOutput *lalog.ByteLogWriter // Store standard output and error from PhantomJS executable
	jsProcCmd     *exec.Cmd            // Headless server process
	jsProcMutex   *sync.Mutex          // Protect against concurrent access to server process
	logger        lalog.Logger
}

// Produce javascript code for browser server and then launch its process in background.
func (instance *Instance) Start() error {
	// Instance is an internal API, hence its parameters are not validated before use.
	instance.jsProcMutex = new(sync.Mutex)
	// Keep latest 512 bytes of standard error and standard output from javascript server
	instance.jsDebugOutput = lalog.NewByteLogWriter(ioutil.Discard, 512)
	instance.Tag = strconv.FormatInt(atomic.AddInt64(&TagCounter, 1), 10)
	instance.logger = lalog.Logger{
		ComponentName: "phantomjs",
		ComponentID:   []lalog.LoggerIDField{{Key: "Created", Value: time.Now().Format(time.Kitchen)}, {Key: "Tag", Value: instance.Tag}},
	}
	// Store server javascript into a temporary file
	var err error
	instance.serverJSFile, err = os.OpenFile(filepath.Join(os.TempDir(), fmt.Sprintf("laitos-phantomjs-%d.js", time.Now().UnixNano())), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("phantomjs.Instance.Start: failed to create temporary file for PhantomJS code - %v", err)
	}
	if _, err := instance.serverJSFile.Write([]byte(fmt.Sprintf(JSCodeTemplate, instance.RenderImagePath, instance.Port))); err != nil {
		return fmt.Errorf("phantomjs.Instance.Instance.Start: failed to write PhantomJS server code - %v", err)
	} else if err := instance.serverJSFile.Sync(); err != nil {
		return fmt.Errorf("phantomjs.Instance.Instance.Start: failed to write PhantomJS server code - %v", err)
	} else if err := instance.serverJSFile.Close(); err != nil {
		return fmt.Errorf("phantomjs.Instance.Instance.Start: failed to write PhantomJS server code - %v", err)
	}
	instance.jsProcCmd = exec.Command(instance.PhantomJSExecPath, "--ssl-protocol=any", "--ignore-ssl-errors=yes", instance.serverJSFile.Name())
	instance.jsProcCmd.Stdout = instance.jsDebugOutput
	instance.jsProcCmd.Stderr = instance.jsDebugOutput
	//instance.jsProcCmd.Stdout = os.Stderr
	//instance.jsProcCmd.Stderr = os.Stderr
	processErrChan := make(chan error, 1)
	go func() {
		if err := instance.jsProcCmd.Start(); err != nil {
			processErrChan <- err
		}
	}()
	// Expect server process to remain running for at least a second for a successful start
	select {
	case err := <-processErrChan:
		return fmt.Errorf("phantomjs.Instance..Start: PhantomJS process failed - %v", err)
	case <-time.After(1 * time.Second):
	}
	// Unconditionally kill the server process after a period of time
	go func() {
		select {
		case err := <-processErrChan:
			instance.logger.Info("Start", "", err, "PhantomJS process has quit")
		case <-time.After(time.Duration(instance.AutoKillTimeoutSec) * time.Second):
		}
		instance.Kill()
	}()
	// Keep knocking on the server port until it is open
	var portIsOpen bool
	for i := 0; i < 20; i++ {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(instance.Port), 2*time.Second)
		if err == nil {
			portIsOpen = true
			conn.Close()
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !portIsOpen {
		instance.Kill()
		return errors.New("phantomjs.Instance..Start: javascript server is not ready")
	}
	return nil
}

// GetDebugOutput retrieves the latest standard output and standard error content from javascript server.
func (instance *Instance) GetDebugOutput() string {
	if instance.jsDebugOutput == nil {
		return ""
	}
	return string(instance.jsDebugOutput.Retrieve(true))
}

// Send a control request via HTTP to the browser server, optionally deserialise the response into receiver.
func (instance *Instance) SendRequest(actionName string, params map[string]interface{}, jsonReceiver interface{}) (err error) {
	body := url.Values{}
	for k, v := range params {
		body[k] = []string{fmt.Sprint(v)}
	}
	// The web server PhantomJS comes with is implemented in Javascript and does not properly handle URL encoding
	fixSpaceForBody := strings.Replace(body.Encode(), "+", "%20", -1)

	resp, err := inet.DoHTTP(inet.HTTPRequest{
		Method: http.MethodPost,
		Body:   strings.NewReader(fixSpaceForBody),
	}, fmt.Sprintf("http://127.0.0.1:%d/%s", instance.Port, actionName))

	// Deserialise the response only if everything is all right
	if err == nil {
		if err = resp.Non2xxToError(); err == nil {
			if jsonReceiver != nil {
				if jsonErr := json.Unmarshal(resp.Body, &jsonReceiver); jsonErr != nil {
					err = fmt.Errorf("slimerjs.Instance.SendRequest: - %v", jsonErr)
				}
			}
		}
	}

	// In case of error, avoid logging HTTP output twice in the log entry.
	if err == nil {
		instance.logger.Info("SendRequest", "", err, "%s(%s)", actionName, fixSpaceForBody)
	} else {
		instance.logger.Info("SendRequest", "", nil, "%s(%s) - %s", actionName, fixSpaceForBody, string(resp.Body))
	}
	return
}

// SetRenderArea sets the rectangular area (within or out of view port) for the next captured page screen shot.
func (instance *Instance) SetRenderArea(top, left, width, height int) error {
	// Ensure input parameters are in the valid range
	if top < 0 {
		top = 0
	}
	if left < 0 {
		left = 0
	}
	if width < 0 {
		width = 10
	}
	if height < 0 {
		height = 10
	}
	return instance.SendRequest("redraw_area", map[string]interface{}{"top": top, "left": left, "width": width, "height": height}, nil)
}

// Tell browser to render page and wait up to 3 seconds for render to finish.
func (instance *Instance) RenderPage() error {
	if err := os.Remove(instance.RenderImagePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := instance.SendRequest("redraw", nil, nil); err != nil {
		return err
	}
	var fileSize int64
	var unchanging int
	for i := 0; i < 60; i++ {
		// See whether image file is already being written into
		if info, err := os.Stat(instance.RenderImagePath); err == nil && info.Size() > 0 {
			if fileSize == info.Size() {
				unchanging++
				if unchanging >= 4 {
					// If size looks identical to last four checks, the render is considered done.
					return nil
				}
			} else {
				// Size is changing, so render is not yet completed.
				unchanging = 0
				fileSize = info.Size()
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("slimerjs.Instance.RenderPage: render is not completed")
}

// Kill browser server process and delete rendered web page image.
func (instance *Instance) Kill() {
	if jsProcCmd := instance.jsProcCmd; jsProcCmd != nil {
		if proc := jsProcCmd.Process; proc != nil {
			instance.logger.Info("Kill", "", nil, "killing process PID %d", proc.Pid)
			if !platform.KillProcess(proc) {
				instance.logger.Warning("Kill", "", nil, "failed to kill process")
			}
		}
		if err := os.Remove(instance.RenderImagePath); err != nil && !os.IsNotExist(err) {
			instance.logger.Warning("Kill", "", err, "failed to delete rendered web page at \"%s\"", instance.RenderImagePath)
		}
		if serverJSFile := instance.serverJSFile; serverJSFile != nil {
			if err := os.Remove(serverJSFile.Name()); err != nil && !os.IsNotExist(err) {
				instance.logger.Warning("Kill", "", err, "failed to delete temporary javascript code \"%s\"", serverJSFile.Name())
			}
		}
	}
	instance.serverJSFile = nil
	instance.jsProcCmd = nil
}

// GoBack navigates browser backward in history.
func (instance *Instance) GoBack() error {
	return instance.SendRequest("back", nil, nil)
}

// GoForward navigates browser forward in history.
func (instance *Instance) GoForward() error {
	return instance.SendRequest("forward", nil, nil)
}

// Reload reloads the current page.
func (instance *Instance) Reload() error {
	return instance.SendRequest("reload", nil, nil)
}

// GoTo navigates to a new URL.
func (instance *Instance) GoTo(userAgent, pageURL string, width, height int) error {
	if !strings.HasPrefix(pageURL, "http://") && !strings.HasPrefix(pageURL, "https://") {
		return errors.New("Instance.GoTo: input URL must begin with http or https scheme")
	}
	var result bool
	err := instance.SendRequest("goto", map[string]interface{}{
		"user_agent":  userAgent,
		"page_url":    pageURL,
		"view_width":  width,
		"view_height": height,
	}, &result)
	if err != nil {
		return err
	}
	if !result {
		return errors.New("Instance.GoTo: result is false")
	}
	return nil
}

const (
	PointerTypeClick   = "click"     // PointerTypeClick is the phantomJS mouse action for clicking.
	PointerTypeMove    = "mousemove" // PointerTypeClick is the phantomJS mouse action for moving pointer.
	PointerButtonLeft  = "left"      // PointerTypeClick is the phantomJS left mouse button.
	PointerButtonRight = "right"     // PointerTypeClick is the phantomJS right mouse button.
)

// Pointer sends pointer to move/click at a coordinate.
func (instance *Instance) Pointer(actionType, button string, x, y int) error {
	return instance.SendRequest("pointer", map[string]interface{}{
		"type":   actionType,
		"x":      x,
		"y":      y,
		"button": button,
	}, nil)
}

const (
	// KeyCodeBackspace is the PhantomJS keyboard key code for the backspace key, identical to SlimerJS.
	KeyCodeBackspace = 16777219
	// KeyCodeEnter is the PhantomJS keyboard key code for Enter key. Enter key only works on PhantomJS, and Return key only works on PhantomJS.
	KeyCodeEnter = 16777221
)

// SendKey either sends a key string or a key code into the currently focused element on page.
func (instance *Instance) SendKey(aString string, aCode int64) error {
	if aString != "" {
		return instance.SendRequest("type", map[string]interface{}{"key_string": aString}, nil)
	} else if aCode != 0 {
		return instance.SendRequest("type", map[string]interface{}{"key_code": strconv.FormatInt(aCode, 10)}, nil)
	}
	return nil
}

// RemotePageInfo describes the title and URL of the phantomJS page.
type RemotePageInfo struct {
	Title string `json:"title"`
	URL   string `json:"page_url"`
}

// GetPageInfo returns title and URL of the current page.
func (instance *Instance) GetPageInfo() (info RemotePageInfo, err error) {
	err = instance.SendRequest("info", nil, &info)
	return
}

// LOReset (line-oriented browser) resets recorded element information so that next DOM navigation will find the first element on page.
func (instance *Instance) LOResetNavigation() error {
	return instance.SendRequest("lo_reset", nil, nil)
}

// ElementInfo tells about an element encountered while navigating around DOM in line-oriented browser.
type ElementInfo struct {
	TagName   string      `json:"tag"`   // TagName is the HTML tag name.
	ID        string      `json:"id"`    // ID is DOM element's ID.
	Name      string      `json:"name"`  // Name is DOM element's name.
	Value     interface{} `json:"value"` // Value is DOM element's value.
	InnerHTML string      `json:"inner"` // InnerHTML is DOM element's inner HTML.
}

// LONext (line-oriented browser) navigates to the next element in DOM. Return information of previous, current, and next element after the action.
func (instance *Instance) LONextElement() (elements []ElementInfo, err error) {
	elements = make([]ElementInfo, 3)
	err = instance.SendRequest("lo_next", nil, &elements)
	return
}

// LONext (line-oriented browser) navigates to the previous element in DOM.  Return information of previous, current, and next element after the action.
func (instance *Instance) LOPreviousElement() (elements []ElementInfo, err error) {
	elements = make([]ElementInfo, 3)
	err = instance.SendRequest("lo_prev", nil, &elements)
	return
}

// LONext (line-oriented browser) navigates to the previous element in DOM. Return information of next N elements.
func (instance *Instance) LONextNElements(n int) (elements []ElementInfo, err error) {
	elements = make([]ElementInfo, 0, n)
	err = instance.SendRequest("lo_next_n", map[string]interface{}{"n": n}, &elements)
	return
}

// LONext (line-oriented browser) sends pointer to click/move to at coordinate of the currently focused element.
func (instance *Instance) LOPointer(actionType, button string) error {
	return instance.SendRequest("lo_pointer", map[string]interface{}{
		"type":   actionType,
		"button": button,
	}, nil)
}

// LONext (line-oriented browser) sets the value of currently focused element.
func (instance *Instance) LOSetValue(value string) error {
	return instance.SendRequest("lo_set_val", map[string]interface{}{"value": value}, nil)
}
