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
	rootDirArgKey      = "oms.RootDir"         // root directory, expected subdir: html
	modelDirArgKey     = "oms.ModelDir"        // models directory, if relative then must be relative to root directory
	listenArgKey       = "oms.Listen"          // address to listen, default: localhost:4040
	listenShortKey     = "l"                   // address to listen (short form)
	logRequestArgKey   = "oms.LogRequest"      // if true then log http request
	apiOnlyArgKey      = "oms.ApiOnly"         // if true then API only web-service, no UI
	uiLangsArgKey      = "oms.Languages"       // list of supported languages
	encodingArgKey     = "oms.CodePage"        // code page for converting source files, e.g. windows-1252
	pageSizeAgrKey     = "oms.MaxRowCount"     // max number of rows to return from read parameters or output tables
	doubleFormatArgKey = "oms.DoubleFormat"    // format to convert float or double value to string, e.g. %.15g
)

// front-end UI subdirectory with html and javascript
const htmlSubDir = "html"

// matcher to find UI supported language corresponding to request
var uiLangMatcher language.Matcher

// if true then log http requests
var isLogRequest bool

// default "page" size: row count to read parameters or output tables
var pageMaxSize int64 = 100

// format to convert float or double value to string
var doubleFmt string = "%.15g"

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
	_ = flag.Int64(pageSizeAgrKey, pageMaxSize, "max number of rows to return from read parameters or output tables")
	_ = flag.String(doubleFormatArgKey, doubleFmt, "format to convert float or double value to string")

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
	pageMaxSize = runOpts.Int64(pageSizeAgrKey, pageMaxSize)
	doubleFmt = runOpts.String(doubleFormatArgKey)

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

	apiGetRoutes(router)    // web-service /api routes to get metadata
	apiReadRoutes(router)   // web-service /api routes to read values
	apiUpdateRoutes(router) // web-service /api routes to update metadata

	// set web root handler: UI web pages or "not found" if this is web-service mode
	if !isApiOnly {
		router.Get("/*", homeHandler, logRequest) // serve UI web pages
	} else {
		router.Get("/*", http.NotFound) // only /api, any other pages not found
	}

	addr := runOpts.String(listenArgKey)
	omppLog.Log("Listen at " + addr)
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

// add http GET web-service /api routes to get metadata
func apiGetRoutes(router *vestigo.Router) {

	//
	// GET model definition
	//

	// GET /api/model-list
	// GET /api/model-list/
	router.Get("/api/model-list", modelListHandler, logRequest)
	router.Get("/api/model-list/", modelListHandler, logRequest)

	// GET /api/model-list-text?lang=en
	// GET /api/model-list-text/
	// GET /api/model-list/text
	// GET /api/model-list/text/lang/:lang
	router.Get("/api/model-list-text", modelTextListHandler, logRequest)
	router.Get("/api/model-list-text/", modelTextListHandler, logRequest)
	router.Get("/api/model-list/text", modelTextListHandler, logRequest)
	router.Get("/api/model-list/text/lang/:lang", modelTextListHandler, logRequest)

	// GET /api/model?model=modelNameOrDigest
	// GET /api/model/:model
	router.Get("/api/model", modelMetaHandler, logRequest)
	router.Get("/api/model/:model", modelMetaHandler, logRequest)

	// GET /api/model-text?model=modelNameOrDigest&lang=en
	// GET /api/model/:model/text
	// GET /api/model/:model/text/lang/:lang
	router.Get("/api/model-text", modelTextHandler, logRequest)
	router.Get("/api/model/:model/text", modelTextHandler, logRequest)
	router.Get("/api/model/:model/text/lang/:lang", modelTextHandler, logRequest)

	// GET /api/model-text-all?model=modelNameOrDigest
	// GET /api/model/:model/text/all
	router.Get("/api/model-text-all", modelAllTextHandler, logRequest)
	router.Get("/api/model/:model/text/all", modelAllTextHandler, logRequest)

	//
	// GET model extra: languages, groups, profile(s)
	//

	// GET /api/lang-list?model=modelNameOrDigest
	// GET /api/model/:model/lang-list
	router.Get("/api/lang-list", langListHandler, logRequest)
	router.Get("/api/model/:model/lang-list", langListHandler, logRequest)

	// GET /api/model-group?model=modelNameOrDigest
	// GET /api/model/:model/group
	router.Get("/api/model-group", modelGroupHandler, logRequest)
	router.Get("/api/model/:model/group", modelGroupHandler, logRequest)

	// GET /api/model-group-text?model=modelNameOrDigest&lang=en
	// GET /api/model/:model/group/text
	// GET /api/model/:model/group/text/lang/:lang
	router.Get("/api/model-group-text", modelGroupTextHandler, logRequest)
	router.Get("/api/model/:model/group/text", modelGroupTextHandler, logRequest)
	router.Get("/api/model/:model/group/text/lang/:lang", modelGroupTextHandler, logRequest)

	// GET /api/model-group-text-all?model=modelNameOrDigest
	// GET /api/model/:model/group/text/all
	router.Get("/api/model-group-text-all", modelGroupAllTextHandler, logRequest)
	router.Get("/api/model/:model/group/text/all", modelGroupAllTextHandler, logRequest)

	// GET /api/model-profile?model=modelNameOrDigest&profile=profileName
	// GET /api/model/:model/profile/:profile
	router.Get("/api/model-profile", modelProfileHandler, logRequest)
	router.Get("/api/model/:model/profile/:profile", modelProfileHandler, logRequest)

	//
	// GET model run results
	//

	// GET /api/run-list?model=modelNameOrDigest
	// GET /api/model/:model/run-list
	router.Get("/api/run-list", runListHandler, logRequest)
	router.Get("/api/model/:model/run-list", runListHandler, logRequest)

	// GET /api/run-list-text?model=modelNameOrDigest&lang=en
	// GET /api/model/:model/run-list/text
	// GET /api/model/:model/run-list/text/lang/:lang
	router.Get("/api/run-list-text", runListTextHandler, logRequest)
	router.Get("/api/model/:model/run-list/text", runListTextHandler, logRequest)
	router.Get("/api/model/:model/run-list/text/lang/:lang", runListTextHandler, logRequest)

	// GET /api/run-status?model=modelNameOrDigest&run=runNameOrDigest
	// GET /api/model/:model/run/:run/status
	router.Get("/api/run-status", runStatusHandler, logRequest)
	router.Get("/api/model/:model/run/:run/status", runStatusHandler, logRequest)

	// GET /api/run-first-status?model=modelNameOrDigest
	// GET /api/model/:model/run/status/first
	router.Get("/api/run-first-status", firstRunStatusHandler, logRequest)
	router.Get("/api/model/:model/run/status/first", firstRunStatusHandler, logRequest)

	// GET /api/run-last-status?model=modelNameOrDigest
	// GET /api/model/:model/run/status/last
	router.Get("/api/run-last-status", lastRunStatusHandler, logRequest)
	router.Get("/api/model/:model/run/status/last", lastRunStatusHandler, logRequest)

	// GET /api/run-last-completed-status?model=modelNameOrDigest
	// GET /api/model/:model/run/status/last/completed
	router.Get("/api/run-last-completed-status", lastCompletedRunStatusHandler, logRequest)
	router.Get("/api/model/:model/run/status/last/completed", lastCompletedRunStatusHandler, logRequest)

	// GET /api/run-text?model=modelNameOrDigest&run=runNameOrDigest&lang=en
	// GET /api/model/:model/run/:run/text
	// GET /api/model/:model/run/:run/text/
	// GET /api/model/:model/run/:run/text/lang/
	// GET /api/model/:model/run/:run/text/lang/:lang
	router.Get("/api/run-text", runTextHandler, logRequest)
	router.Get("/api/model/:model/run/:run/text", runTextHandler, logRequest)
	router.Get("/api/model/:model/run/:run/text/", runTextHandler, logRequest)
	router.Get("/api/model/:model/run/:run/text/lang/", runTextHandler, logRequest)
	router.Get("/api/model/:model/run/:run/text/lang/:lang", runTextHandler, logRequest)

	// GET /api/run-text-all?model=modelNameOrDigest&run=runNameOrDigest
	// GET /api/model/:model/run/:run/text/all
	router.Get("/api/run-text-all", runAllTextHandler, logRequest)
	router.Get("/api/model/:model/run/:run/text/all", runAllTextHandler, logRequest)

	// GET /api/run-last-completed-text?model=modelNameOrDigest&lang=en
	// GET /api/model/:model/run/last/completed/text
	// GET /api/model/:model/run/last/completed/text/lang/:lang
	router.Get("/api/run-last-completed-text", lastCompletedRunTextHandler, logRequest)
	router.Get("/api/model/:model/run/last/completed/text", lastCompletedRunTextHandler, logRequest)
	router.Get("/api/model/:model/run/last/completed/text/lang/:lang", lastCompletedRunTextHandler, logRequest)

	// GET /api/run-last-completed-text-all?model=modelNameOrDigest
	// GET /api/model/:model/run/last/completed/text/all
	router.Get("/api/run-last-completed-text-all", lastCompletedRunAllTextHandler, logRequest)
	router.Get("/api/model/:model/run/last/completed/text/all", lastCompletedRunAllTextHandler, logRequest)

	//
	// GET model set of input parameters (workset)
	//

	// GET /api/workset-list?model=modelNameOrDigest
	// GET /api/model/:model/workset-list
	router.Get("/api/workset-list", worksetListHandler, logRequest)
	router.Get("/api/model/:model/workset-list", worksetListHandler, logRequest)

	// GET /api/workset-list-text?model=modelNameOrDigest&lang=en
	// GET /api/model/:model/workset-list/text
	// GET /api/model/:model/workset-list/text/lang/:lang
	router.Get("/api/workset-list-text", worksetListTextHandler, logRequest)
	router.Get("/api/model/:model/workset-list/text", worksetListTextHandler, logRequest)
	router.Get("/api/model/:model/workset-list/text/lang/:lang", worksetListTextHandler, logRequest)

	// GET /api/workset-status?model=modelNameOrDigest&set=setName
	// GET /api/model/:model/workset/:set/status
	router.Get("/api/workset-status", worksetStatusHandler, logRequest)
	router.Get("/api/model/:model/workset/:set/status", worksetStatusHandler, logRequest)

	// GET /api/workset-default-status?model=modelNameOrDigest
	// GET /api/model/:model/workset/status/default
	router.Get("/api/workset-default-status", worksetDefaultStatusHandler, logRequest)
	router.Get("/api/model/:model/workset/status/default", worksetDefaultStatusHandler, logRequest)

	// GET /api/workset-text?model=modelNameOrDigest&set=setName&lang=en
	// GET /api/model/:model/workset/:set/text
	// GET /api/model/:model/workset/:set/text/
	// GET /api/model/:model/workset/:set/text/lang/
	// GET /api/model/:model/workset/:set/text/lang/:lang
	router.Get("/api/workset-text", worksetTextHandler, logRequest)
	router.Get("/api/model/:model/workset/:set/text", worksetTextHandler, logRequest)
	router.Get("/api/model/:model/workset/:set/text/", worksetTextHandler, logRequest)
	router.Get("/api/model/:model/workset/:set/text/lang/", worksetTextHandler, logRequest)
	router.Get("/api/model/:model/workset/:set/text/lang/:lang", worksetTextHandler, logRequest)

	// GET /api/workset-text-all?model=modelNameOrDigest&set=setName
	// GET /api/model/:model/workset/:set/text/all
	router.Get("/api/workset-text-all", worksetAllTextHandler, logRequest)
	router.Get("/api/model/:model/workset/:set/text/all", worksetAllTextHandler, logRequest)
}

// add http GET or POST web-service /api routes to read parameters or output tables
func apiReadRoutes(router *vestigo.Router) {

	// POST /api/model/:model/workset/:set/parameter/value
	router.Post("/api/model/:model/workset/:set/parameter/value", worksetParameterPageReadHandler, logRequest)

	// POST /api/model/:model/workset/:set/parameter/value-id
	router.Post("/api/model/:model/workset/:set/parameter/value-id", worksetParameterIdPageReadHandler, logRequest)

	// POST /api/model/:model/run/:run/parameter/value
	router.Post("/api/model/:model/run/:run/parameter/value", runParameterPageReadHandler, logRequest)

	// POST /api/model/:model/run/:run/parameter/value-id
	router.Post("/api/model/:model/run/:run/parameter/value-id", runParameterIdPageReadHandler, logRequest)

	// POST /api/model/:model/run/:run/table/value
	router.Post("/api/model/:model/run/:run/table/value", runTablePageReadHandler, logRequest)

	// POST /api/model/:model/run/:run/table/value-id
	router.Post("/api/model/:model/run/:run/table/value-id", runTableIdPageReadHandler, logRequest)

	// GET /api/workset-parameter-value?model=modelNameOrDigest&set=setName&name=parameterName
	// GET /api/workset-parameter-value?model=modelNameOrDigest&set=setName&name=parameterName&start=0
	// GET /api/workset-parameter-value?model=modelNameOrDigest&set=setName&name=parameterName&start=0&count=100
	// GET /api/model/:model/workset/:set/parameter/:name/value
	// GET /api/model/:model/workset/:set/parameter/:name/value/start/:start
	// GET /api/model/:model/workset/:set/parameter/:name/value/start/:start/count/:count
	router.Get("/api/workset-parameter-value", worksetParameterPageGetHandler, logRequest)
	router.Get("/api/model/:model/workset/:set/parameter/:name/value", worksetParameterPageGetHandler, logRequest)
	router.Get("/api/model/:model/workset/:set/parameter/:name/value/start/:start", worksetParameterPageGetHandler, logRequest)
	router.Get("/api/model/:model/workset/:set/parameter/:name/value/start/:start/count/:count", worksetParameterPageGetHandler, logRequest)

	// GET /api/run-parameter-value?model=modelNameOrDigest&run=runNameOrDigest&name=parameterName
	// GET /api/run-parameter-value?model=modelNameOrDigest&run=runNameOrDigest&name=parameterName&start=0
	// GET /api/run-parameter-value?model=modelNameOrDigest&run=runNameOrDigest&name=parameterName&start=0&count=100
	// GET /api/model/:model/run/:run/parameter/:name/value
	// GET /api/model/:model/run/:run/parameter/:name/value/start/:start
	// GET /api/model/:model/run/:run/parameter/:name/value/start/:start/count/:count
	router.Get("/api/run-parameter-value", runParameterPageGetHandler, logRequest)
	router.Get("/api/model/:model/run/:run/parameter/:name/value", runParameterPageGetHandler, logRequest)
	router.Get("/api/model/:model/run/:run/parameter/:name/value/start/:start", runParameterPageGetHandler, logRequest)
	router.Get("/api/model/:model/run/:run/parameter/:name/value/start/:start/count/:count", runParameterPageGetHandler, logRequest)

	// GET /api/workset-parameter-csv?model=modelNameOrDigest&set=setName&name=parameterName&bom=true
	// GET /api/model/:model/workset/:set/parameter/:name/csv
	router.Get("/api/workset-parameter-csv", worksetParameterCsvGetHandler, logRequest)
	router.Get("/api/model/:model/workset/:set/parameter/:name/csv", worksetParameterCsvGetHandler, logRequest)

	// GET /api/model/:model/workset/:set/parameter/:name/csv-bom
	router.Get("/api/model/:model/workset/:set/parameter/:name/csv-bom", worksetParameterCsvBomGetHandler, logRequest)

	// GET /api/workset-parameter-csv-id?model=modelNameOrDigest&set=setName&name=parameterName&bom=true
	// GET /api/model/:model/workset/:set/parameter/:name/csv-id
	router.Get("/api/workset-parameter-csv-id", worksetParameterIdCsvGetHandler, logRequest)
	router.Get("/api/model/:model/workset/:set/parameter/:name/csv-id", worksetParameterIdCsvGetHandler, logRequest)

	// GET /api/model/:model/workset/:set/parameter/:name/csv-id-bom
	router.Get("/api/model/:model/workset/:set/parameter/:name/csv-id-bom", worksetParameterIdCsvBomGetHandler, logRequest)

	// GET /api/run-parameter-csv?model=modelNameOrDigest&run=runNameOrDigest&name=parameterName&bom=true
	// GET /api/model/:model/run/:run/parameter/:name/csv
	router.Get("/api/run-parameter-csv", runParameterCsvGetHandler, logRequest)
	router.Get("/api/model/:model/run/:run/parameter/:name/csv", runParameterCsvGetHandler, logRequest)

	// GET /api/model/:model/run/:run/parameter/:name/csv-bom
	router.Get("/api/model/:model/run/:run/parameter/:name/csv-bom", runParameterCsvBomGetHandler, logRequest)

	// GET /api/run-parameter-csv-id?model=modelNameOrDigest&run=runNameOrDigest&name=parameterName&bom=true
	// GET /api/model/:model/run/:run/parameter/:name/csv-id
	router.Get("/api/run-parameter-csv-id", runParameterIdCsvGetHandler, logRequest)
	router.Get("/api/model/:model/run/:run/parameter/:name/csv-id", runParameterIdCsvGetHandler, logRequest)

	// GET /api/model/:model/run/:run/parameter/:name/csv-id-bom
	router.Get("/api/model/:model/run/:run/parameter/:name/csv-id-bom", runParameterIdCsvBomGetHandler, logRequest)

	// GET /api/run-table-expr?model=modelNameOrDigest&run=runNameOrDigest&name=tableName&start=0
	// GET /api/run-table-expr?model=modelNameOrDigest&run=runNameOrDigest&name=tableName&start=0&count=100
	// GET /api/model/:model/run/:run/table/:name/expr
	// GET /api/model/:model/run/:run/table/:name/expr/start/:start
	// GET /api/model/:model/run/:run/table/:name/expr/start/:start/count/:count
	router.Get("/api/run-table-expr", runTableExprPageGetHandler, logRequest)
	router.Get("/api/model/:model/run/:run/table/:name/expr", runTableExprPageGetHandler, logRequest)
	router.Get("/api/model/:model/run/:run/table/:name/expr/start/:start", runTableExprPageGetHandler, logRequest)
	router.Get("/api/model/:model/run/:run/table/:name/expr/start/:start/count/:count", runTableExprPageGetHandler, logRequest)

	// GET /api/run-table-acc?model=modelNameOrDigest&run=runNameOrDigest&name=tableName
	// GET /api/run-table-acc?model=modelNameOrDigest&run=runNameOrDigest&name=tableName&start=0
	// GET /api/run-table-acc?model=modelNameOrDigest&run=runNameOrDigest&name=tableName&start=0&count=100
	// GET /api/model/:model/run/:run/table/:name/acc
	// GET /api/model/:model/run/:run/table/:name/acc/start/:start
	// GET /api/model/:model/run/:run/table/:name/acc/start/:start/count/:count
	router.Get("/api/run-table-acc", runTableAccPageGetHandler, logRequest)
	router.Get("/api/model/:model/run/:run/table/:name/acc", runTableAccPageGetHandler, logRequest)
	router.Get("/api/model/:model/run/:run/table/:name/acc/start/:start", runTableAccPageGetHandler, logRequest)
	router.Get("/api/model/:model/run/:run/table/:name/acc/start/:start/count/:count", runTableAccPageGetHandler, logRequest)

	// GET /api/run-table-all-acc?model=modelNameOrDigest&run=runNameOrDigest&name=tableName
	// GET /api/run-table-all-acc?model=modelNameOrDigest&run=runNameOrDigest&name=tableName&start=0
	// GET /api/run-table-all-acc?model=modelNameOrDigest&run=runNameOrDigest&name=tableName&start=0&count=100
	// GET /api/model/:model/run/:run/table/:name/all-acc
	// GET /api/model/:model/run/:run/table/:name/all-acc/start/:start
	// GET /api/model/:model/run/:run/table/:name/all-acc/start/:start/count/:count
	router.Get("/api/run-table-all-acc", runTableAllAccPageGetHandler, logRequest)
	router.Get("/api/model/:model/run/:run/table/:name/all-acc", runTableAllAccPageGetHandler, logRequest)
	router.Get("/api/model/:model/run/:run/table/:name/all-acc/start/:start", runTableAllAccPageGetHandler, logRequest)
	router.Get("/api/model/:model/run/:run/table/:name/all-acc/start/:start/count/:count", runTableAllAccPageGetHandler, logRequest)
}

// add web-service /api routes to update metadata
func apiUpdateRoutes(router *vestigo.Router) {

	// POST /api/workset-readonly?model=modelNameOrDigest&set=setName&readonly=true
	// POST /api/model/:model/workset/:set/readonly/:readonly
	router.Post("/api/workset-readonly", worksetReadonlyUpdateHandler, logRequest)
	router.Post("/api/model/:model/workset/:set/readonly/:readonly", worksetReadonlyUpdateHandler, logRequest)

	// DELETE /api/model/:model/workset/:set
	// POST /api/model/:model/workset/:set/delete
	// POST /api/workset/delete?model=modelNameOrDigest&set=setName
	router.Delete("/api/model/:model/workset/:set", worksetDeleteHandler, logRequest)
	router.Post("/api/model/:model/workset/:set/delete", worksetDeleteHandler, logRequest)
	router.Post("/api/workset/delete", worksetDeleteHandler, logRequest)

	// PUT /api/workset-new
	// POST /api/workset-new
	router.Put("/api/workset-new", worksetReplaceHandler, logRequest)
	router.Post("/api/workset-new", worksetReplaceHandler, logRequest)

	// PATCH /api/workset
	// POST /api/workset
	router.Patch("/api/workset", worksetMergeHandler, logRequest)
	router.Post("/api/workset", worksetMergeHandler, logRequest)

	// DELETE /api/model/:model/workset/:set/parameter/:name
	// POST /api/model/:model/workset/:set/parameter/:name/delete
	// POST /api/workset-parameter/delete?model=modelNameOrDigest&set=setName&parameter=name
	router.Delete("/api/model/:model/workset/:set/parameter/:name", worksetParameterDeleteHandler, logRequest)
	router.Post("/api/model/:model/workset/:set/parameter/:name/delete", worksetParameterDeleteHandler, logRequest)
	router.Post("/api/workset-parameter/delete", worksetParameterDeleteHandler, logRequest)
}