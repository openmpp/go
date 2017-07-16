// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"errors"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/husobee/vestigo"
	"golang.org/x/text/language"

	"go.openmpp.org/ompp/config"
	"go.openmpp.org/ompp/omppLog"
)

// config keys to get values from ini-file or command line arguments.
const (
	rootDirArgKey    = "oms.RootDir"    // root directory, expected subdir: html
	modelDirArgKey   = "oms.ModelDir"   // models directory, if relative then must be relative to root directory
	listenArgKey     = "oms.Listen"     // address to listen, default: localhost:4040
	listenShortKey   = "l"              // address to listen (short form)
	logRequestArgKey = "oms.LogRequest" // if true then log http request
	apiOnlyArgKey    = "oms.ApiOnly"    // if true then API only web-service, no UI
	uiLangsArgKey    = "oms.Languages"  // list of supported languages
	encodingArgKey   = "oms.CodePage"   // code page for converting source files, e.g. windows-1252
)

// front-end UI subdirectory with html and javascript
const htmlSubDir = "html"

// matcher to find UI supported language corresponding to request
var uiLangMatcher language.Matcher

// if true then log http requests
var isLogRequest bool

// main entry point: wrapper to handle errors
func main() {
	defer exitOnPanic() // fatal error handler: log and exit

	err := mainBody(os.Args)
	if err != nil {
		omppLog.Log(err.Error())
		os.Exit(1)
	}
	omppLog.Log("Done.") // compeleted OK
}

// actual main body
func mainBody(args []string) error {

	// set command line argument keys and ini-file keys
	_ = flag.String(rootDirArgKey, "", "root directory, default: current directory")
	_ = flag.String(modelDirArgKey, "models/bin", "models directory, if relative then must be relative to root directory")
	_ = flag.String(listenArgKey, "localhost:4040", "address to listen")
	_ = flag.String(listenShortKey, "localhost:4040", "address to listen (short form of "+listenArgKey+")")
	_ = flag.Bool(logRequestArgKey, false, "if true then log HTTP requests")
	_ = flag.Bool(apiOnlyArgKey, false, "if true then API only web-service, no UI")
	_ = flag.String(uiLangsArgKey, "en", "comma-separated list of supported languages")
	_ = flag.String(encodingArgKey, "", "code page to convert source file into utf-8, e.g.: windows-1252")

	// pairs of full and short argument names to map short name to full name
	var optFs = []config.FullShort{
		config.FullShort{Full: listenArgKey, Short: listenShortKey},
	}

	// parse command line arguments and ini-file
	runOpts, logOpts, extraArgs, err := config.New(encodingArgKey, optFs)
	if err != nil {
		return errors.New("Invalid arguments: " + err.Error())
	}
	if len(extraArgs) > 0 {
		return errors.New("Invalid arguments: " + strings.Join(extraArgs, " "))
	}
	isLogRequest = runOpts.Bool(logRequestArgKey)
	isApiOnly := runOpts.Bool(apiOnlyArgKey)
	rootDir := runOpts.String(rootDirArgKey) // server root directory

	// if UI required then server root directory must have html subdir
	if !isApiOnly {
		htmlDir := filepath.Join(rootDir, htmlSubDir)
		if err := isDirExist(htmlDir); err != nil {
			return err
		}
	}

	// change to root directory
	if rootDir != "" && rootDir != "." {
		if err := os.Chdir(rootDir); err != nil {
			return errors.New("Error: unable to change directory: " + err.Error())
		}
	}
	omppLog.New(logOpts) // adjust log options, log path can be relative to root directory

	if rootDir != "" && rootDir != "." {
		omppLog.Log("Changing directory to: ", rootDir)
	}

	// model directory required to build initial list of model sqlite files
	modelDir := runOpts.String(modelDirArgKey)
	if modelDir == "" {
		return errors.New("Error: model directory argument cannot be empty")
	}
	omppLog.Log("Model directory: ", modelDir)

	if err := theCatalog.RefreshSqlite(modelDir); err != nil {
		return err
	}

	// set UI languages to find model text in browser language
	ll := strings.Split(runOpts.String(uiLangsArgKey), ",")
	var lt []language.Tag
	for _, ls := range ll {
		if ls != "" {
			lt = append(lt, language.Make(ls))
		}
	}
	if len(lt) <= 0 {
		lt = append(lt, language.English)
	}
	uiLangMatcher = language.NewMatcher(lt)

	// setup router and start server
	router := vestigo.NewRouter()

	// GET /api/model-list
	// GET /api/model-list/
	router.Get("/api/model-list", modelListHandler, logRequest)
	router.Get("/api/model-list/", modelListHandler, logRequest)

	// GET /api/model-list-text?lang=en
	// GET /api/model-list-text/
	// GET /api/model-list-text/:lang
	router.Get("/api/model-list-text", modelTextListHandler, logRequest)
	router.Get("/api/model-list-text/", modelTextListHandler, logRequest)
	router.Get("/api/model-list-text/:lang", modelTextListHandler, logRequest)

	// GET /api/model?dn=a1b2c3d
	// GET /api/model?dn=modelName
	// GET /api/model/:dn
	router.Get("/api/model", modelMetaHandler, logRequest)
	router.Get("/api/model/:dn", modelMetaHandler, logRequest)

	// GET /api/model-text?dn=a1b2c3d
	// GET /api/model-text?dn=modelName&lang=en
	// GET /api/model-text/:dn
	// GET /api/model-text/:dn/:lang
	router.Get("/api/model-text", modelTextHandler, logRequest)
	router.Get("/api/model-text/:dn", modelTextHandler, logRequest)
	router.Get("/api/model-text/:dn/:lang", modelTextHandler, logRequest)

	// GET /api/lang-list?dn=a1b2c3d
	// GET /api/lang-list?dn=modelName
	// GET /api/lang-list/:dn
	router.Get("/api/lang-list", langListHandler, logRequest)
	router.Get("/api/lang-list/:dn", langListHandler, logRequest)

	// GET /api/model-group?dn=a1b2c3d
	// GET /api/model-group?dn=modelName
	// GET /api/model-group/:dn
	router.Get("/api/model-group", modelGroupHandler, logRequest)
	router.Get("/api/model-group/:dn", modelGroupHandler, logRequest)

	// GET /api/model-group-text?dn=a1b2c3d
	// GET /api/model-group-text?dn=modelName&lang=en
	// GET /api/model-group-text/:dn
	// GET /api/model-group-text/:dn/:lang
	router.Get("/api/model-group-text", modelGroupTextHandler, logRequest)
	router.Get("/api/model-group-text/:dn", modelGroupTextHandler, logRequest)
	router.Get("/api/model-group-text/:dn/:lang", modelGroupTextHandler, logRequest)

	// GET /api/model-profile?digest=a1b2c3d?name=profileName
	// GET /api/model-profile/:digest/:name
	router.Get("/api/model-profile", modelProfileHandler, logRequest)
	router.Get("/api/model-profile/:digest/:name", modelProfileHandler, logRequest)

	// GET /api/run-status?dn=a1b2c3d&rdn=1f2e3d4
	// GET /api/run-status?dn=modelName&rdn=runName
	// GET /api/run/:dn/:rdn
	router.Get("/api/run-status", runStatusHandler, logRequest)
	router.Get("/api/run-status/:dn/:rdn", runStatusHandler, logRequest)

	// GET /api/first-run-status?dn=a1b2c3d
	// GET /api/first-run-status?dn=modelName
	// GET /api/first-run/:dn
	router.Get("/api/first-run-status", firstRunStatusHandler, logRequest)
	router.Get("/api/first-run-status/:dn", firstRunStatusHandler, logRequest)

	// GET /api/last-run-status?dn=a1b2c3d
	// GET /api/last-run-status?dn=modelName
	// GET /api/last-run/:dn
	router.Get("/api/last-run-status", lastRunStatusHandler, logRequest)
	router.Get("/api/last-run-status/:dn", lastRunStatusHandler, logRequest)

	// GET /api/last-completed-run?dn=a1b2c3d
	// GET /api/last-completed-run?dn=modelName?lang=en
	// GET /api/last-completed-run/:dn
	// GET /api/last-completed-run/:dn/:lang
	router.Get("/api/last-completed-run", lastCompletedRunHandler, logRequest)
	router.Get("/api/last-completed-run/:dn", lastCompletedRunHandler, logRequest)
	router.Get("/api/last-completed-run/:dn/:lang", lastCompletedRunHandler, logRequest)

	// set web root handler: UI web pages or "not found" if this is web-service mode
	if !isApiOnly {
		router.Get("/*", homeHandler, logRequest) // serve UI web pages
	} else {
		router.Get("/*", http.NotFound) // only /api, any other pages not found
	}

	addr := runOpts.String(listenArgKey)
	omppLog.Log("Starting at " + addr)
	if !isApiOnly {
		omppLog.Log("To start open in your browser: " + addr)
	}
	omppLog.Log("To finish press Ctrl+C")

	err = http.ListenAndServe(addr, router)
	return err
}

// exitOnPanic log error message and exit with return = 2
func exitOnPanic() {
	r := recover()
	if r == nil {
		return // not in panic
	}
	switch e := r.(type) {
	case error:
		omppLog.Log(e.Error())
	case string:
		omppLog.Log(e)
	default:
		omppLog.Log("FAILED")
	}
	os.Exit(2) // final exit
}

// isDirExist return error if directory does not exist or not accessible
func isDirExist(dirPath string) error {
	stat, err := os.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("Error: directory not exist: " + dirPath)
		}
		return errors.New("Error: unable to access directory: " + dirPath + " : " + err.Error())
	}
	if !stat.IsDir() {
		return errors.New("Error: directory expected: " + dirPath)
	}
	return nil
}
