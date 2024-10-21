// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"net/http"
	"strconv"

	"github.com/openmpp/go/ompp/db"
	"github.com/openmpp/go/ompp/helper"
	"github.com/openmpp/go/ompp/omppLog"
)

// return service configuration: model catalog, run catalog, job service and disk use configuartion.
//
//	GET /api/service/config
func serviceConfigHandler(w http.ResponseWriter, r *http.Request) {

	st := struct {
		OmsName        string             // server instance name
		DoubleFmt      string             // format to convert float or double value to string
		LoginUrl       string             // user login URL for UI
		LogoutUrl      string             // user logout URL for UI
		AllowUserHome  bool               // if true then store user settings in home directory
		AllowDownload  bool               // if true then allow download from home/io/download directory
		AllowUpload    bool               // if true then allow upload from home/io/upload directory
		AllowFiles     bool               // if true then allow user files, if home directory specified then files directory: home/io
		AllowMicrodata bool               // if true then allow model run microdata
		IsJobControl   bool               // if true then job control enabled
		IsModelDoc     bool               // if true then model documentation is enabled
		IsDiskUse      bool               // if true then storage usage control enabled
		IsDiskCleanup  bool               // if true then disk cleanup enabled
		DiskUse        diskUseConfig      // disk use config
		Env            map[string]string  // server config environmemt variables for UI
		UiExtra        string             // UI extra config from etc/ui.extra.json
		ModelCatalog   ModelCatalogConfig // "public" state of model catalog
		RunCatalog     RunCatalogConfig   // "public" state of model run catalog
	}{
		OmsName:        theCfg.omsName,
		DoubleFmt:      theCfg.doubleFmt,
		AllowUserHome:  theCfg.isHome,
		AllowDownload:  theCfg.downloadDir != "",
		AllowUpload:    theCfg.uploadDir != "",
		AllowFiles:     theCfg.filesDir != "",
		AllowMicrodata: theCfg.isMicrodata,
		IsJobControl:   theCfg.isJobControl,
		IsModelDoc:     theCfg.docDir != "",
		IsDiskUse:      theCfg.isDiskUse,
		Env:            theCfg.env,
		UiExtra:        theCfg.uiExtra,
		ModelCatalog:   theCatalog.toPublicConfig(),
		RunCatalog:     *theRunCatalog.toPublicConfig(),
	}
	if theCfg.isDiskUse {
		_, st.DiskUse = theRunCatalog.getDiskUseStatus()
		st.IsDiskCleanup = st.DiskUse.dbCleanupCmd != "" && theCfg.dbcopyPath != ""
	}

	jsonResponse(w, r, st)
}

// return job service state: model runs queue, active runs and run history
//
//	GET /api/service/state
func serviceStateHandler(w http.ResponseWriter, r *http.Request) {

	// service state: model run jobs queue, active jobs, history jobs and compute servers state
	type cItem struct {
		Name       string     // name of server or cluster
		State      string     // state: start, stop, ready, error, off
		TotalRes   ComputeRes // total computational resources (CPU cores and memory)
		UsedRes    ComputeRes // resources (CPU cores and memory) used by all oms instances
		OwnRes     ComputeRes // resources (CPU cores and memory) used by this instance
		ErrorCount int        // number of incomplete starts, stops and errors
		LastUsedTs int64      // last time for model run (unix milliseconds)
	}
	st := struct {
		IsJobControl    bool             // if true then job control enabled
		JobServiceState                  // jobs service state: paused, resources usage and limits
		Queue           []RunJob         // list of model run jobs in the queue
		Active          []RunJob         // list of active (currently running) model run jobs
		History         []historyJobFile // history of model runs
		ComputeState    []cItem          // state of computational servers or clusters
		IsDiskUse       bool             // if true then storage usage control enabled
		IsDiskCleanup   bool             // if true then disk cleanup enabled
		IsDiskOver      bool             // if true then storage use reach the limit
		diskUseConfig                    // storage use settings
	}{
		IsJobControl: theCfg.isJobControl,
		Queue:        []RunJob{},
		Active:       []RunJob{},
		History:      []historyJobFile{},
		ComputeState: []cItem{},
		IsDiskUse:    theCfg.isDiskUse,
	}

	if theCfg.isJobControl {
		jsState, qKeys, qJobs, aKeys, aJobs, hKeys, hJobs, cState := theRunCatalog.getRunJobs()

		st.JobServiceState = jsState

		st.Queue = make([]RunJob, len(qKeys))
		for k := range qKeys {
			st.Queue[k] = qJobs[k]
			st.Queue[k].Env = map[string]string{}
		}

		st.Active = make([]RunJob, len(aKeys))
		for k := range aKeys {
			st.Active[k] = aJobs[k]
			st.Active[k].Pid = 0
			st.Active[k].CmdPath = ""
			st.Active[k].LogPath = ""
			st.Active[k].Env = map[string]string{}
		}

		st.History = make([]historyJobFile, len(hKeys))
		for k := range hKeys {
			st.History[k] = hJobs[k]
		}

		st.ComputeState = make([]cItem, len(cState))
		for k := range cState {
			st.ComputeState[k].Name = cState[k].name
			st.ComputeState[k].State = cState[k].state
			if st.ComputeState[k].State == "" {
				st.ComputeState[k].State = "off"
			}
			st.ComputeState[k].TotalRes = cState[k].totalRes
			st.ComputeState[k].UsedRes = cState[k].usedRes
			st.ComputeState[k].OwnRes = cState[k].ownRes
			st.ComputeState[k].ErrorCount = cState[k].errorCount
			st.ComputeState[k].LastUsedTs = cState[k].lastUsedTs
		}
	}

	if theCfg.isDiskUse {
		st.IsDiskOver, st.diskUseConfig = theRunCatalog.getDiskUseStatus()
		st.IsDiskCleanup = st.dbCleanupCmd != "" && theCfg.dbcopyPath != ""
	}

	jsonResponse(w, r, st)
}

// return disk use state: summary of disk use and list of model database files size.
//
//	GET /api/service/disk-use
func serviceDiskUseHandler(w http.ResponseWriter, r *http.Request) {

	st := struct {
		IsDiskUse bool         // if true then storage usage control enabled
		DiskUse   diskUseState // storage space use state
		DbDiskUse []dbDiskUse  // model db file disk usage
	}{
		IsDiskUse: theCfg.isDiskUse,
		DbDiskUse: []dbDiskUse{},
	}

	if theCfg.isDiskUse {
		st.DiskUse, st.DbDiskUse = theRunCatalog.getDiskUse()
	}
	jsonResponse(w, r, st)
}

// refersh disk use state: scan disk usage now.
//
//	POST /api/service/disk-use/refresh
func serviceRefreshDiskUseHandler(w http.ResponseWriter, r *http.Request) {

	if theCfg.isDiskUse {
		refreshDiskScanC <- true
	}
	// respond with disk usage active status
	w.Header().Set("Content-Location", "/api/service/disk-use/refresh/"+strconv.FormatBool(theCfg.isDiskUse))
}

// job control state, log file content and run progress
type runJobState struct {
	JobStatus string      // if not empty then job run status name: success, error, exit
	RunJob                // job control state: job control file content
	RunStatus []db.RunPub // if not empty then run_lst and run_progerss from db
	Lines     []string    // log file content
}

// return empty value of job control state
func emptyRunJobState(submitStamp string) runJobState {
	return runJobState{
		RunJob: RunJob{
			SubmitStamp: submitStamp,
			RunRequest: RunRequest{
				Opts:   map[string]string{},
				Env:    map[string]string{},
				Tables: []string{},
				Microdata: struct {
					IsToDb     bool
					IsInternal bool
					Entity     []struct {
						Name string
						Attr []string
					}
				}{
					Entity: []struct {
						Name string
						Attr []string
					}{},
				},
				RunNotes: []struct {
					LangCode string
					Note     string
				}{}},
		},
		RunStatus: []db.RunPub{},
		Lines:     []string{},
	}
}

// return active job state, run log file content and, if model run exists in database then also run progress
//
//	GET /api/service/job/active/:job
func jobActiveHandler(w http.ResponseWriter, r *http.Request) {

	// url or query parameters: submission stamp
	submitStamp := getRequestParam(r, "job")
	if submitStamp == "" {
		http.Error(w, "Invalid (empty) submission stamp", http.StatusBadRequest)
		return
	}

	// find job state in run catalog
	aj, isOk := theRunCatalog.getActiveJobItem(submitStamp)

	if !isOk || aj.isError {
		jsonResponse(w, r, emptyRunJobState(submitStamp)) // job not found or job control file error
		return
	}

	// get job control state, read log file and run progress, if it is available
	isOk, st := getJobState(aj.filePath)
	if !isOk {
		jsonResponse(w, r, emptyRunJobState(submitStamp)) // unable to read job control file
		return
	}

	jsonResponse(w, r, st) // return final result
}

// return queue job state
//
//	GET /api/service/job/queue/:job
func jobQueueHandler(w http.ResponseWriter, r *http.Request) {

	// url or query parameters: submission stamp
	submitStamp := getRequestParam(r, "job")
	if submitStamp == "" {
		http.Error(w, "Invalid (empty) submission stamp", http.StatusBadRequest)
		return
	}

	// find job state in run catalog
	qj, isOk := theRunCatalog.getQueueJobItem(submitStamp)

	if !isOk || qj.isError {
		jsonResponse(w, r, emptyRunJobState(submitStamp)) // job not found or job control file error
		return
	}

	// get job control state, log file and run progress are always empty
	isOk, st := getJobState(qj.filePath)
	if !isOk {
		jsonResponse(w, r, emptyRunJobState(submitStamp)) // unable to read job control file
		return
	}

	jsonResponse(w, r, st) // return final result
}

// return history job state, run log file content and, if model run exists in database then also return run progress
//
//	GET /api/service/job/history/:job
func jobHistoryHandler(w http.ResponseWriter, r *http.Request) {

	// url or query parameters: submission stamp
	submitStamp := getRequestParam(r, "job")
	if submitStamp == "" {
		http.Error(w, "Invalid (empty) submission stamp", http.StatusBadRequest)
		return
	}

	// find job state in run catalog
	hj, isOk := theRunCatalog.getHistoryJobItem(submitStamp)

	if !isOk || hj.isError {
		jsonResponse(w, r, emptyRunJobState(submitStamp)) // job not found or job control file error
		return
	}

	// get job control state, read log file and run progress, if it is available
	isOk, st := getJobState(hj.filePath)
	if !isOk {
		jsonResponse(w, r, emptyRunJobState(submitStamp)) // unable to read job control file
		return
	}
	st.JobStatus = hj.JobStatus

	jsonResponse(w, r, st) // return final result
}

// returns job control file content, run log file content and, if model run exists in database then also return run progress
// it is clear (set to empty) server-only part of job state: pid, exe path, log path and environment
func getJobState(filePath string) (bool, *runJobState) {

	st := emptyRunJobState("")

	// read job control file
	var jc RunJob
	isOk, err := helper.FromJsonFile(filePath, &jc)
	if err != nil {
		omppLog.Log(err)
	}
	if !isOk || err != nil {
		return false, &st
	}

	// set job state and clear server-only part of the job state: pid, exe path, log path and environment
	st.RunJob = jc
	st.Pid = 0
	st.CmdPath = ""
	st.LogPath = ""
	st.Env = map[string]string{}
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

	// read log file content
	if jc.LogPath != "" {
		st.Lines, _ = readLogFile(jc.LogPath)
	}

	// get run progress if model run exist in database
	if jc.ModelDigest != "" && jc.RunStamp != "" {
		if _, isOk = theCatalog.ModelDicByDigest(jc.ModelDigest); isOk { // if model exist
			if rst, isOk := theCatalog.RunStatusList(jc.ModelDigest, jc.RunStamp); isOk { // get run status
				st.RunStatus = rst
			}
		}
	}

	return true, &st // return final result
}

// move job into the specified queue index position.
//
//	PUT /api/service/job/move/:pos/:job
//
// Top of the queue position is zero, negative position treated as zero.
// If position number exceeds queue length then job moved to the bottom of the queue.
func jobMoveHandler(w http.ResponseWriter, r *http.Request) {

	// url or query parameters: position and submission stamp
	sp := getRequestParam(r, "pos")
	nPos, err := strconv.Atoi(sp)
	if sp == "" || err != nil {
		http.Error(w, "Invalid (or empty) job queue position", http.StatusBadRequest)
		return
	}

	submitStamp := getRequestParam(r, "job")
	if submitStamp == "" {
		http.Error(w, "Invalid (empty) submission stamp", http.StatusBadRequest)
		return
	}

	// move job in the queue and rename files in the queue
	isOk, fileMoveLst := theRunCatalog.moveJobInQueue(submitStamp, nPos)

	for _, fm := range fileMoveLst {
		fileMoveAndLog(false, fm[0], fm[1])
	}

	w.Header().Set("Content-Type", "text/plain")
	if !isOk {
		w.Header().Set("Content-Location", "service/job/move/false/"+sp+"/"+submitStamp)
		return
	}
	// else: job moved into the specified queue position
	w.Header().Set("Content-Location", "service/job/move/true/"+sp+"/"+submitStamp)
}

// delete only job history json file, it does not delete model run.
//
//	DELETE /api/service/job/delete/history/:job
func jobHistoryDeleteHandler(w http.ResponseWriter, r *http.Request) {

	// url or query parameters: submission stamp
	submitStamp := getRequestParam(r, "job")
	if submitStamp == "" {
		http.Error(w, "Invalid (empty) submission stamp", http.StatusBadRequest)
		return
	}

	// find job history in run catalog
	hj, isOk := theRunCatalog.getHistoryJobItem(submitStamp)
	if isOk {

		isOk = fileDeleteAndLog(true, hj.filePath)
		if !isOk {
			http.Error(w, "Unable to delete job file", http.StatusInternalServerError)
			return
		}
	}
	// job history file deleted or job history not found
	w.Header().Set("Content-Location", "/api/service/job/delete/history/"+submitStamp)
}

// delete all successful or not successful jobs history json files.
// it does not delete model runs.
//
//	DELETE /api/service/job/delete/history-all/:success
func jobHistoryAllDeleteHandler(w http.ResponseWriter, r *http.Request) {

	// url or query parameters: successful or not boolean flag
	sp := getRequestParam(r, "success")
	isSuccess, err := strconv.ParseBool(sp)
	if sp == "" || err != nil {
		http.Error(w, "Invalid (or empty) history delete flag, expected true or false", http.StatusBadRequest)
		return
	}

	doJobHistoryAllDelete(isSuccess, w)
}

// delete all successful or not successful jobs history json files, it does not delete model runs.
func doJobHistoryAllDelete(isSuccess bool, w http.ResponseWriter) {

	nDel := 0
	if theCfg.isJobControl {

		_, _, _, _, _, hKeys, hJobs, _ := theRunCatalog.getRunJobs()

		for k := range hKeys {

			if isSuccess && hJobs[k].JobStatus != "success" || !isSuccess && hJobs[k].JobStatus == "success" {
				continue
			}
			if isOk := fileDeleteAndLog(true, hJobs[k].filePath); !isOk {
				http.Error(w, "Unable to delete job file "+hJobs[k].SubmitStamp, http.StatusInternalServerError)
				return
			}
			nDel++
		}
	}

	// all job history file deleted
	if isSuccess {
		w.Header().Set("Content-Location", "/api/service/job/delete/history-all-success/"+strconv.Itoa(nDel))
		return
	} // else
	w.Header().Set("Content-Location", "/api/service/job/delete/history-all-not-success/"+strconv.Itoa(nDel))
}
