package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

const (
	MethodPagePrintToPDF = "Page.printToPDF"
	ReadBufferSize       = 20000
)

var (
	RemoteDebuggingPort = 9222
)

type RemoteDebuggingEndpoint struct {
	WebSocketDebuggingUrl string `json:"webSocketDebuggerUrl"`
}

type PagePrintToPDFParams struct {
	Landscape           bool `json:"landscape"`
	DisplayHeaderFooter bool `json:"displayHeaderFooter"`
}

type PagePrintToPDFResponse struct {
	Result map[string]string `json:"result"`
}

type DevToolsCall struct {
	Id     int         `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params"`
	Result interface{} `json:"result"`
}

type DevToolsResponse struct {
	Id     int         `json:"id"`
	Result interface{} `json:"result"`
}

func printToPDF(targetFile, outputFile string) error {
	var chromeHeadlessCmd string

	switch runtime.GOOS {
	case "darwin":
		if cmdPath, err := exec.LookPath("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"); err == nil {
			chromeHeadlessCmd = cmdPath
		}
	}

	cdir, _ := os.Getwd()
	c := exec.Command(chromeHeadlessCmd, "--headless", "--disable-gpu", "--remote-debugging-port="+strconv.Itoa(RemoteDebuggingPort), targetFile)
	c.Dir = cdir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Start(); err != nil {
		return err
	}
	defer c.Process.Kill()
	time.Sleep(1 * time.Second)

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/json", RemoteDebuggingPort), nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	var debuggingEndpoints []RemoteDebuggingEndpoint
	if err := json.NewDecoder(res.Body).Decode(&debuggingEndpoints); err != nil {
		return err
	}

	var wsUrl string
	for _, v := range debuggingEndpoints {
		if v.WebSocketDebuggingUrl != "" {
			wsUrl = v.WebSocketDebuggingUrl
		}
	}
	if wsUrl == "" {
		return errors.New("can not find devtools endpoint")
	}

	devToolsConn, _, err := websocket.DefaultDialer.Dial(wsUrl, nil)
	if err != nil {
		return err
	}

	r := &DevToolsCall{
		Id:     1,
		Method: MethodPagePrintToPDF,
		Params: &PagePrintToPDFParams{
			DisplayHeaderFooter: false,
		},
	}
	if err := devToolsConn.WriteJSON(r); err != nil {
		return err
	}

	var ResponsePrintToPDF PagePrintToPDFResponse
	if err := devToolsConn.ReadJSON(&ResponsePrintToPDF); err != nil {
		return err
	}

	pdfBuf, err := base64.StdEncoding.DecodeString(ResponsePrintToPDF.Result["data"])
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(outputFile, pdfBuf, 0755); err != nil {
		return err
	}

	return nil
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: html2pdf [file] [outputfile]")
		os.Exit(1)
	}

	targetFile := os.Args[1]
	outputFile := os.Args[2]
	if err := printToPDF(targetFile, outputFile); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
