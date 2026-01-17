// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"bufio"
	"io"
	"net/http"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/openmpp/go/ompp/helper"
	"github.com/openmpp/go/ompp/omppLog"
)

// reload models catalog: rescan models directory tree and reload model.sqlite.
//
//	POST /api/admin/all-models/refresh
func allModelsRefreshHandler(w http.ResponseWriter, r *http.Request) {

	lang := preferedRequestLang(r, "") // get prefered language for messages

	// model directory required to build list of model sqlite files
	modelLogDir, _ := theCatalog.getModelLogDir()
	modelDir, _ := theCatalog.getModelDir()
	if modelDir == "" {
		omppLog.Log("Failed to refresh models catalog: path to model directory cannot be empty")
		http.Error(w, helper.MsgL(lang, "Failed to refresh models catalog: path to model directory cannot be empty"), http.StatusBadRequest)
		return
	}
	omppLog.Log("Model directory:", modelDir)

	// refresh models catalog
	if err := theCatalog.refreshSqlite(modelDir, modelLogDir); err != nil {
		omppLog.LogNoLT(err)
		http.Error(w, helper.MsgL(lang, "Failed to refersh models catalog:", modelDir), http.StatusBadRequest)
		return
	}

	// refresh run state catalog
	if err := theRunCatalog.refreshCatalog(theCfg.etcDir, nil); err != nil {
		omppLog.LogNoLT(err)
		http.Error(w, helper.MsgL(lang, "Failed to refersh model runs catalog"), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Location", "/api/admin/all-models/refresh/"+filepath.ToSlash(modelDir))
	w.Header().Set("Content-Type", "text/plain")
}

// clean models catalog: close all model.sqlite connections and clean models catalog
//
//	POST /api/admin/all-models/close
func allModelsCloseHandler(w http.ResponseWriter, r *http.Request) {

	lang := preferedRequestLang(r, "") // get prefered language for messages

	// close models catalog
	modelDir, _ := theCatalog.getModelDir()

	if err := theCatalog.closeAll(); err != nil {
		omppLog.LogNoLT(err)
		http.Error(w, helper.MsgL(lang, "Failed to close models catalog:", modelDir), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Location", "/api/admin/all-models/close/"+filepath.ToSlash(modelDir))
	w.Header().Set("Content-Type", "text/plain")
}

// close model.sqlite connection and clean model from catalog
//
//	POST /api/admin/model/:model/close
//
// Model identified by digest-or-name.
// If multiple models with same name exist then result is undefined.
func modelCloseHandler(w http.ResponseWriter, r *http.Request) {

	dn := getRequestParam(r, "model")
	lang := preferedRequestLang(r, "") // get prefered language for messages

	if dn == "" {
		omppLog.Log("Error: invalid (empty) model digest and name")
		http.Error(w, helper.MsgL(lang, "Invalid (empty) model digest and name"), http.StatusBadRequest)
		return
	}

	// close model and remove from catalog
	if _, _, err := theCatalog.closeModel(dn); err != nil {
		omppLog.LogNoLT(err)
		http.Error(w, helper.MsgL(lang, "Failed to close model", ": ", dn), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Location", "/api/admin/model/"+dn+"/close")
	w.Header().Set("Content-Type", "text/plain")
}

// delete all model files from disk
//
//	POST /api/admin/model/:model/delete
//
// Model identified by digest-or-name.
// If multiple models with same name exist then result is undefined.
func modelDeleteHandler(w http.ResponseWriter, r *http.Request) {

	dn := getRequestParam(r, "model")
	lang := preferedRequestLang(r, "") // get prefered language for messages

	if dn == "" {
		omppLog.Log("Error: invalid (empty) model digest and name")
		http.Error(w, helper.MsgL(lang, "Invalid (empty) model digest and name"), http.StatusBadRequest)
		return
	}

	// close model and delete all model files from disk
	if err := theCatalog.deleteModel(dn); err != nil {
		omppLog.LogNoLT(err)
		http.Error(w, helper.MsgL(lang, "Failed to delete model:", dn), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Location", "/api/admin/model/"+dn+"/delete")
	w.Header().Set("Content-Type", "text/plain")
}

// open SQLite db file and get all models from it.
//
//	POST /api/admin/db-file-open/:path
//
// Path to model database must be relative to models/bin root.
// Slashes / or back \ slashes in the path must be replaced with * star.
// If model(s) with the same digest already open then method return an error.
func modelOpenDbFileHandler(w http.ResponseWriter, r *http.Request) {

	dbPath := getRequestParam(r, "path")
	lang := preferedRequestLang(r, "") // get prefered language for messages

	if dbPath == "" {
		omppLog.Log("Error: invalid (empty) path to model database file")
		http.Error(w, helper.MsgL(lang, "Invalid (empty) path to model database file"), http.StatusBadRequest)
		return
	}
	dbPath = strings.ReplaceAll(dbPath, "*", "/") // restore slashed / path

	// make db path relative to models/bin root
	// and check if model database file is already open: it should not be in the list of model db files
	mbinDir, _ := theCatalog.getModelDir()
	srcPath := path.Join(mbinDir, dbPath)

	mbs := theCatalog.allModels()
	if slices.IndexFunc(mbs, func(mb modelBasic) bool { return mb.relPath == srcPath }) >= 0 {
		http.Error(w, helper.MsgL(lang, "Error: model database file already open:", dbPath), http.StatusBadRequest)
		return
	}

	// open db file and add models to catalog
	n, err := theCatalog.loadModelDbFile(srcPath)
	if err != nil {
		omppLog.LogNoLT(err)
		http.Error(w, helper.MsgL(lang, "Failed to open model db file:", dbPath), http.StatusBadRequest)
		return
	}
	if n <= 0 {
		omppLog.LogNoLT(err)
		http.Error(w, helper.MsgL(lang, "Error: invalid (empty) model db file:", dbPath), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Location", "/api/admin/db-file-open/"+filepath.ToSlash(dbPath))
	w.Header().Set("Content-Type", "text/plain")
}

// pause or resume jobs queue processing by this oms instance
//
//	POST /api/admin/jobs-pause/:pause
func jobsPauseHandler(w http.ResponseWriter, r *http.Request) {
	doJobsPause(jobQueuePausedPath(theCfg.omsName), "/api/admin/jobs-pause/", w, r)
}

// pause or resume jobs queue processing by all oms instances
//
//	POST /api/admin-all/jobs-pause/:pause
func jobsAllPauseHandler(w http.ResponseWriter, r *http.Request) {
	doJobsPause(jobAllQueuePausedPath(), "/api/admin-all/jobs-pause/", w, r)
}

// Pause or resume jobs queue processing by this oms instance all by all oms instances
//
//	POST /api/admin/jobs-pause/:pause
//	POST /api/admin-all/jobs-pause/:pause
func doJobsPause(filePath, msgPath string, w http.ResponseWriter, r *http.Request) {

	lang := preferedRequestLang(r, "") // get prefered language for messages

	// url or query parameters: pause or resume boolean flag
	sp := getRequestParam(r, "pause")
	isPause, err := strconv.ParseBool(sp)
	if sp == "" || err != nil {
		http.Error(w, helper.MsgL(lang, "Invalid (or empty) jobs pause flag, expected true or false"), http.StatusBadRequest)
		return
	}

	// create jobs paused state file or remove it to resume queue processing
	isOk := false
	if isPause {
		isOk = fileCreateEmpty(false, filePath)
	} else {
		isOk = fileDeleteAndLog(false, filePath)
	}
	if !isOk {
		isPause = !isPause // operation failed
	}

	// Content-Location: /api/admin/jobs-pause/true
	w.Header().Set("Content-Location", msgPath+strconv.FormatBool(isPause))
	w.Header().Set("Content-Type", "text/plain")
}

// async start of model database cleanup and retrun LogFileName on success
//
//	POST /api/admin/db-cleanup/:path
//	POST /api/admin/db-cleanup/:path/name/:name
//	POST /api/admin/db-cleanup/:path/name/:name/digest/:digest
//	POST /api/admin/db-cleanup/:path/lang/:lang
//
// Relative path to model database file is required, slash / in the path must be replaced with * star.
// Model name and digest are optional parameters.
// Cleanup is done on separate thread by db cleanup script, defined in disk.ini [Common] DbCleanup.
// Model database must be closed, for example by: POST /api/admin/model/:model/close.
func modelDbCleanupHandler(w http.ResponseWriter, r *http.Request) {

	// if disk space use control disabled then do nothing
	if !theCfg.isDiskUse {
		w.Header().Set("Content-Location", "/api/admin/db-cleanup/none")
		w.Header().Set("Content-Type", "text/plain")
	}

	// validate parameters: path to database file is required
	dbPath := getRequestParam(r, "path")
	name := getRequestParam(r, "name")
	digest := getRequestParam(r, "digest")
	lang := preferedRequestLang(r, "lang") // get prefered language for dbcopy log messages

	if dbPath == "" {
		omppLog.Log("Error: invalid (empty) path to model database file")
		http.Error(w, helper.MsgL(lang, "Invalid (empty) path to model database file"), http.StatusBadRequest)
		return
	}
	dbPath = strings.ReplaceAll(dbPath, "*", "/") // restore slashed / path

	// check if database cleanup script defined
	// check if database file is exists and belong to current oms instance: it must be in the list of instance database files
	diskUse, dbUse := theRunCatalog.getDiskUse()

	if diskUse.dbCleanupCmd == "" {
		omppLog.Log("Error: db cleanup script is not defined in disk.ini")
		http.Error(w, helper.MsgL(lang, "Error: db cleanup script is not defined in disk.ini"), http.StatusInternalServerError)
		return
	}
	if i := slices.IndexFunc(
		dbUse, func(du dbDiskUse) bool { return du.DbPath == dbPath }); i < 0 || i >= len(dbUse) {
		http.Error(w, helper.MsgL(lang, "Error: model database not found", name, digest), http.StatusBadRequest)
		return
	}

	// check if model database is closed: it should not be in the list of model db files
	mbs := theCatalog.allModels()

	if i := slices.IndexFunc(mbs, func(mb modelBasic) bool { return mb.relPath == dbPath }); i >= 0 && i < len(mbs) {
		http.Error(w, helper.MsgL(lang, "Error: model database must be closed", name, digest), http.StatusBadRequest)
		return
	}

	// join db path with models/bin root
	srcPath := dbPath
	if mr, isOk := theCatalog.getModelDir(); isOk {
		srcPath = filepath.Join(mr, dbPath)
	}
	srcPath = filepath.Clean(srcPath)

	// make log file name and path
	ln := filepath.Base(dbPath)
	if ln == "." || ln == "/" || ln == "\\" {
		ln = "no-name"
	}
	ld, _ := theCatalog.getModelLogDir()

	ln, lp := dbCleanupLogNamePath(ln, ld)

	// start database cleanup
	go func(cmdPath, mDbPath, mName, mDigest, msgLang string, logPath string) {

		// make db cleanup command
		if mName == "" && (mDigest != "" || msgLang != "") {
			mName = "no-name"
		}
		if mDigest == "" && msgLang != "" {
			mDigest = "no-digest"
		}
		cArgs := []string{
			mDbPath,
			mName,
			mDigest,
		}
		if msgLang != "" {
			cArgs = append(cArgs, msgLang)
		}
		cmd := exec.Command(cmdPath, cArgs...)

		// connect console output to output log file
		outPipe, err := cmd.StdoutPipe()
		if err != nil {
			omppLog.Log("Error at join to stdout log", ": ", logPath, ": ", err)
			return
		}
		errPipe, err := cmd.StderrPipe()
		if err != nil {
			omppLog.Log("Error at join to stderr log", ": ", logPath, ": ", err)
			return
		}
		outDoneC := make(chan bool, 1)
		errDoneC := make(chan bool, 1)
		logTck := time.NewTicker(logTickTimeout * time.Millisecond)

		// start console output listners
		isLogOk := fileCreateEmpty(false, logPath)
		if !isLogOk {
			omppLog.Log("Error at creating log file", ": ", logPath)
		}

		doLog := func(path string, r io.Reader, done chan<- bool) {
			sc := bufio.NewScanner(r)
			for sc.Scan() {
				if isLogOk {
					isLogOk = writeToCmdLog(path, false, sc.Text())
				}
			}
			done <- true
			close(done)
		}
		go doLog(logPath, outPipe, outDoneC)
		go doLog(logPath, errPipe, errDoneC)

		// start db cleanup
		omppLog.Log(strings.Join(cmd.Args, " "))
		isLogOk = writeToCmdLog(logPath, true, strings.Join(cmd.Args, " "))

		err = cmd.Start()
		if err != nil {
			omppLog.Log("Error at", ": ", logPath, ": ", err)
			writeToCmdLog(logPath, true, err.Error())
			return
		}
		// else db cleanup started: wait until completed

		// wait until stdout and stderr closed
		for outDoneC != nil || errDoneC != nil {
			select {
			case _, ok := <-outDoneC:
				if !ok {
					outDoneC = nil
				}
			case _, ok := <-errDoneC:
				if !ok {
					errDoneC = nil
				}
			case <-logTck.C:
			}
		}

		// wait for db cleanup to be completed
		e := cmd.Wait()
		if e != nil {
			omppLog.Log("Error at: ", cmd.Args)
			writeToCmdLog(logPath, true, e.Error())
			return
		}
		// else: completed OK
		if isLogOk {
			writeToCmdLog(logPath, true, "Done.")
		} else {
			omppLog.Log("Warning: db cleanup log output may be incomplete")
		}
		// refresh disk usage
		refreshDiskScanC <- true

	}(diskUse.dbCleanupCmd, srcPath, name, digest, lang, lp)

	// db cleanup is starting now: return path to log file
	jsonResponse(w, r, struct{ LogFileName string }{LogFileName: ln})
}

// get list of all db cleanup log files
//
//	GET /api/admin/db-cleanup/log-all
func dbCleanupAllLogGetHandler(w http.ResponseWriter, r *http.Request) {

	lang := preferedRequestLang(r, "") // get prefered language for messages

	type fi struct {
		DbName      string // database file name
		LogStamp    string // log file date-time stamp
		LogFileName string // db-cleanup.2024_03_05_00_30_37_568.modelOne.sqlite.console.txt
	}

	logDir, isLog := theCatalog.getModelLogDir()
	if !isLog {
		jsonResponse(w, r, []fi{}) // log is not enabled: empty response
		return
	}

	// get list of models/log/db-cleanup.*.txt files
	fiLst := []fi{}

	pl, err := filepath.Glob(logDir + string(filepath.Separator) + "db-cleanup.*.txt")
	if err != nil {
		http.Error(w, helper.MsgL(lang, "Error at db cleanup log files list"), http.StatusInternalServerError)
		return
	}
	for _, p := range pl {

		ts, base, fn := parseDbCleanupLogPath(p)

		if ts != "" && base != "" {
			fiLst = append(fiLst, fi{
				DbName:      base,
				LogStamp:    ts,
				LogFileName: fn,
			})
		}
	}

	jsonResponse(w, r, fiLst)
}

// get db cleanup log file content by name
//
//	GET /api/admin/db-cleanup/log/:name
func dbCleanupFileLogGetHandler(w http.ResponseWriter, r *http.Request) {

	// check log file: it must be db cleanup log file
	logName := getRequestParam(r, "name")
	lang := preferedRequestLang(r, "") // get prefered language for messages

	ts, base, fn := parseDbCleanupLogPath(logName)
	if ts == "" || base == "" || fn != logName {
		http.Error(w, helper.MsgL(lang, "Invalid db cleanup log file name"), http.StatusBadRequest)
		return
	}

	// response: db cleanup log file info and content
	st := struct {
		DbName      string   // database file name
		LogStamp    string   // log file date-time stamp
		LogFileName string   // db-cleanup.2024_03_05_00_30_37_568.modelOne.sqlite.console.txt
		Size        int64    // bytes, log file size
		ModTs       int64    // unix milliseconds, log file update time
		Lines       []string // log file content
	}{
		Lines: []string{},
	}

	// check if log file exists in models/log directory
	logDir, isLog := theCatalog.getModelLogDir()
	if !isLog {
		jsonResponse(w, r, []string{}) // log is not enabled: empty response
		return
	}
	logPath := filepath.Join(logDir, logName)

	fi, err := helper.FileStat(logPath)
	if err != nil {
		http.Error(w, helper.MsgL(lang, "Error at db cleanup log file get:", err), http.StatusBadRequest)
		return
	}

	// read log file content and return result
	st.DbName = base
	st.LogStamp = ts
	st.LogFileName = logName
	st.Size = fi.Size()
	st.ModTs = fi.ModTime().UnixMilli()
	st.Lines, _ = readLogFile(logPath)

	jsonResponse(w, r, st)
}

// for all oms instances: get state, computational and disk resources usage
//
//	GET /api/admin-all/state
func adminAllStateGetHandler(w http.ResponseWriter, r *http.Request) {

	lang := preferedRequestLang(r, "") // get prefered language for messages

	if !theCfg.isAdminAll {
		http.Error(w, helper.MsgL(lang, "Forbidden: disabled on the server"), http.StatusForbidden)
		return
	}

	ast, jsp, csp := theRunCatalog.getAllAdminPubState()

	st := struct {
		IsAdminAll    bool         // if true then allow global administrative routes: /admin-all/
		IsJobControl  bool         // if true then job control enabled
		IsDiskUse     bool         // if true then storage usage control enabled
		JobServicePub              // jobs service state: paused, resources usage and limits
		ComputeState  []computePub // state of computational servers or clusters
		adminState                 // model run state and resources usage: for global admin only
	}{
		IsAdminAll:    theCfg.isAdminAll,
		IsJobControl:  theCfg.isJobControl,
		IsDiskUse:     theCfg.isDiskUse,
		JobServicePub: jsp,
		ComputeState:  csp,
		adminState:    ast,
	}
	jsonResponse(w, r, st)
}

// for global admin: get queue job state content by oms instance name and submit stamp
//
//	GET /api/admin-all/job/queue/:oms/stamp/:stamp/state
func adminAllJobQueueStateHandler(w http.ResponseWriter, r *http.Request) {

	lang := preferedRequestLang(r, "") // get prefered language for messages

	if !theCfg.isAdminAll {
		http.Error(w, helper.MsgL(lang, "Forbidden: disabled on the server"), http.StatusForbidden)
		return
	}

	// url or query parameters: submission stamp
	oms := getRequestParam(r, "oms")
	if oms == "" {
		http.Error(w, helper.MsgL(lang, "Invalid (empty) oms user name"), http.StatusBadRequest)
		return
	}
	submitStamp := getRequestParam(r, "stamp")
	if submitStamp == "" {
		http.Error(w, helper.MsgL(lang, "Invalid (empty) submission stamp"), http.StatusBadRequest)
		return
	}

	// find queue job request by oms name and submit stamp
	qr, ok := theRunCatalog.findQueueRequest(oms, submitStamp)
	if !ok {
		jsonResponse(w, r, emptyRunJob(submitStamp)) // queue request not found, it may be completed
		return
	}
	// read job control state
	ok, jc := readAllAdminJobState(qr.filePath)
	if !ok {
		jsonResponse(w, r, emptyRunJob(submitStamp)) // unable to read job control file
		return
	}

	jsonResponse(w, r, jc) // return final result
}

// for global admin: get active job state content by oms instance name and submit stamp
//
//	GET /api/admin-all/job/active/:oms/stamp/:stamp/state
func adminAllJobActiveStateHandler(w http.ResponseWriter, r *http.Request) {

	lang := preferedRequestLang(r, "") // get prefered language for messages

	if !theCfg.isAdminAll {
		http.Error(w, helper.MsgL(lang, "Forbidden: disabled on the server"), http.StatusForbidden)
		return
	}

	// url or query parameters: submission stamp
	oms := getRequestParam(r, "oms")
	if oms == "" {
		http.Error(w, helper.MsgL(lang, "Invalid (empty) oms user name"), http.StatusBadRequest)
		return
	}
	submitStamp := getRequestParam(r, "stamp")
	if submitStamp == "" {
		http.Error(w, helper.MsgL(lang, "Invalid (empty) submission stamp"), http.StatusBadRequest)
		return
	}

	// find active job run usage by oms name and submit stamp
	ru, ok := theRunCatalog.findActiveRunUsage(oms, submitStamp)
	if !ok {
		jsonResponse(w, r, emptyRunJob(submitStamp)) // job not found, it may be completed
		return
	}
	// read job control state
	ok, jc := readAllAdminJobState(ru.filePath)
	if !ok {
		jsonResponse(w, r, emptyRunJob(submitStamp)) // unable to read job control file
		return
	}
	// if log log file path is empty for that active run then set log file in active runs list
	if ru.logAbsPath == "" && jc.LogAbsPath != "" {
		theRunCatalog.updateActiveJobLogPath(oms, submitStamp, jc.LogAbsPath)
	}
	jsonResponse(w, r, jc) // return final result
}

// for global admin: get active job log file name and content by oms instance name and submit stamp, response is string array, first line is a file name
//
//	GET /api/admin-all/job/active/:oms/stamp/:stamp/log
func adminAllJobActiveLogHandler(w http.ResponseWriter, r *http.Request) {

	lang := preferedRequestLang(r, "") // get prefered language for messages

	if !theCfg.isAdminAll {
		http.Error(w, helper.MsgL(lang, "Forbidden: disabled on the server"), http.StatusForbidden)
		return
	}

	// url or query parameters: submission stamp
	oms := getRequestParam(r, "oms")
	if oms == "" {
		http.Error(w, helper.MsgL(lang, "Invalid (empty) oms user name"), http.StatusBadRequest)
		return
	}
	submitStamp := getRequestParam(r, "stamp")
	if submitStamp == "" {
		http.Error(w, helper.MsgL(lang, "Invalid (empty) submission stamp"), http.StatusBadRequest)
		return
	}

	// find active job run usage by oms name and submit stamp
	lf := []string{}

	ru, ok := theRunCatalog.findActiveRunUsage(oms, submitStamp)
	if !ok {
		jsonResponse(w, r, lf) // job not found, it may be completed
		return
	}

	//  if log path is empty then read job control file to get log path from it
	if ru.logAbsPath == "" {
		ok, jc := readAllAdminJobState(ru.filePath)
		if !ok {
			jsonResponse(w, r, lf) // unable to read job control file
			return
		}

		// update log log file path in active runs list
		if jc.LogAbsPath != "" {
			theRunCatalog.updateActiveJobLogPath(oms, submitStamp, jc.LogAbsPath)
		}
		ru.logAbsPath = jc.LogAbsPath
	}
	if ru.logAbsPath == "" {
		jsonResponse(w, r, lf) // log path is empty, log disabled
		return
	}

	// read log content and return final result
	lf = append(lf, filepath.Base(ru.logAbsPath)) // first line is log file name

	if lines, ok := readLogFile(ru.logAbsPath); ok {
		lf = append(lf, lines...)
	}
	jsonResponse(w, r, lf)
}

// read job control file content
func readAllAdminJobState(filePath string) (bool, *RunJob) {

	st := emptyRunJob("")
	if filePath == "" {
		return false, &st
	}

	// read job control file
	isOk, err := helper.FromJsonFile(filePath, &st)
	if err != nil {
		omppLog.LogNoLT(err)
	}
	if !isOk || err != nil {
		return false, &st
	}

	// set job state default empty values instead of missing null values
	if len(st.Opts) == 0 {
		st.Opts = map[string]string{}
	}
	if st.Tables == nil {
		st.Tables = []string{}
	}
	if st.Microdata.Entity == nil {
		st.Microdata.Entity = []struct {
			Name string
			Attr []string
		}{}
	}
	if st.RunNotes == nil {
		st.RunNotes = []struct {
			LangCode string
			Note     string
		}{}
	}

	return true, &st // return final result
}

// for global admin: stop queue run: find and delete job control file from the queue
//
//	PUT /api/admin-all/run/stop/queue/:oms/stamp/:stamp
func adminAllStopQueueRunHandler(w http.ResponseWriter, r *http.Request) {

	lang := preferedRequestLang(r, "") // get prefered language for messages

	if !theCfg.isAdminAll {
		http.Error(w, helper.MsgL(lang, "Forbidden: disabled on the server"), http.StatusForbidden)
		return
	}

	// url or query parameters: submission stamp
	oms := getRequestParam(r, "oms")
	if oms == "" {
		http.Error(w, helper.MsgL(lang, "Invalid (empty) oms user name"), http.StatusBadRequest)
		return
	}
	stamp := getRequestParam(r, "stamp")
	if stamp == "" {
		http.Error(w, helper.MsgL(lang, "Invalid (empty) submission stamp"), http.StatusBadRequest)
		return
	}

	// find queue job request by oms name and submit stamp
	qr, isFound := theRunCatalog.findQueueRequest(oms, stamp)
	if !isFound {
		http.Error(w, helper.MsgL(lang, "Model run not found:", oms, stamp), http.StatusBadRequest)
		return // empty result: oms or submission stamp not found in the queue
	}

	isFound = moveJobQueueToFailed(qr.filePath, stamp, qr.ModelName, qr.ModelDigest, stamp, true) // move job control file to history

	// write queue job run key as response
	w.Header().Set("Content-Location", "/api/admin-all/run/stop/queue/"+oms+"/stamp/"+stamp+"/"+strconv.FormatBool(isFound))
	w.Header().Set("Content-Type", "text/plain")
}
