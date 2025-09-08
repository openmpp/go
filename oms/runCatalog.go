// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/openmpp/go/ompp/helper"
)

// RunCatalog is a most recent state of model run for each model.
type RunCatalog struct {
	rscLock         sync.Mutex                         // mutex to lock for model list operations
	models          map[string]modelRunBasic           // map model digest to basic info to run the model and manage log files
	etcDir          string                             // model run templates directory, if relative then must be relative to oms root directory
	runTemplates    []string                           // list of model run templates
	mpiTemplates    []string                           // list of model MPI run templates
	presets         []RunOptionsPreset                 // list of preset run options
	modelRuns       map[string]map[string]*runStateLog // map each model digest to run stamps to run state and log file
	JobServiceState                                    // jobs service state: paused, resources usage and limits
	DiskUse         diskUseState                       // storage space use state
	DbDiskUse       []dbDiskUse                        // db files disk usage, it may be not a model.sqlite but also model.db file
	queueKeys       []string                           // run submission stamps of model runs waiting in the queue
	queueJobs       map[string]queueJobFile            // model run jobs waiting in the queue
	activeJobs      map[string]runJobFile              // active (currently running) model run jobs
	historyJobs     map[string]historyJobFile          // models run jobs history
	selectedKeys    []string                           // jobs selected from queue to run now
	computeState    map[string]computeItem             // map names of server or cluster the state of computational resources
	startupNames    []string                           // names of the servers which are starting now
	shutdownNames   []string                           // names of the servers which are stopping now
	cfgRes          map[string]modelCfgRes             // map model digest to resources configuration
	first           jobHostUse                         // first MPI job host usage
}

var theRunCatalog RunCatalog // list of most recent state of model run for each model.

// modelRunBasic is basic info to run model and obtain model logs
type modelRunBasic struct {
	name     string // model name
	binDir   string // database and .exe directory: directory part of models/bin/model.sqlite
	logDir   string // model log directory
	isLogDir bool   // if true then use model log directory for model run logs
}

// RunCatalogConfig is "public" state of model run catalog for json import-export
type RunCatalogConfig struct {
	RunTemplates       []string           // list of model run templates
	DefaultMpiTemplate string             // default template to run MPI model
	MpiTemplates       []string           // list of model MPI run templates
	Presets            []RunOptionsPreset // list of preset run options
}

// RunOptionsPreset is "public" view of model run options preset
type RunOptionsPreset struct {
	Name    string // name of preset, based on file name
	Options string // run options as json stringify
}

// RunRequest is request to run the model with specified model options.
// Log to console always enabled.
// Model run console output redirected to log file: modelName.YYYY_MM_DD_hh_mm_ss_SSS.console.log
type RunRequest struct {
	ModelName   string            // model name to run
	ModelDigest string            // model digest to run
	RunStamp    string            // run stamp, if empty then auto-generated as timestamp
	Dir         string            // working directory to run the model, if relative then must be relative to oms root directory
	Opts        map[string]string // model run options
	Env         map[string]string // environment variables to set
	Threads     int               // number of modelling threads
	IsMpi       bool              // if true then it use MPI to run the model
	Mpi         struct {
		Np          int  // if non-zero then number of MPI processes
		IsNotOnRoot bool // if true then do no run modelling threads on MPI root process
		IsNotByJob  bool // if true then do not allocate resources by job, use CPU, threads and memory as is
	}
	Template  string   // template file name to make run model command line
	Tables    []string // if not empty then output tables or table groups to retain, by default retain all tables
	Microdata struct {
		IsToDb     bool       // if true then store entity microdata in database: -Microdata.ToDb true
		IsInternal bool       // if true then allow to use internal attributes: -Microdata.UseInternal true
		Entity     []struct { // list of entities and attributes: -Microdata.Person age,income -Microdata.Other All
			Name string   // entity name
			Attr []string // list of microdata attributes, it is also can be All
		}
	}
	RunNotes []struct {
		LangCode string // model language code
		Note     string // run notes
	}
}

// RunJob is model run request and run job control: submission stamp and model process id
type RunJob struct {
	SubmitStamp string // submission timestamp
	Pid         int    // process id
	CmdPath     string // executable path
	RunRequest         // model run request: model name, digest and run options
	Res         RunRes // job run resources: CPU cores and memory
	IsOverLimit bool   // if true then job run resource(s) exceed limit(s)
	QueuePos    int    // one-based position of MPI job in global queue or any (MPI or non-MPI) job in localhost queue
	LogFileName string // log file name
	LogPath     string // log file path: log/dir/modelName.RunStamp.console.log
	IniPath     string // if not empty then actual ini file path, may be relative to log directory
	BinDir      string // if not empty then model run bin directory
	WorkDir     string // if not empty then model run work directory
}

// RunRes is model run computational resources
type RunRes struct {
	ComputeRes       // total resources: cpu count and memory size
	ProcessCount int // computaional proccess count
	ThreadCount  int // computaional thread count
	ProcessMemMb int // if not zero then memory required per proccess in megabytes
	ThreadMemMb  int // if not zero then memory required for each thread in megabytes
}

// Computational resources
type ComputeRes struct {
	Cpu int // cpu cores count
	Mem int // if not zero then memory size in gigabytes
}

// computational resources required to run the model
type modelCfgRes struct {
	Path         string // model bin directory and model name joined by / slash, ex: 1-Rp/RiskPaths
	ProcessMemMb int    // if not zero then memory required per proccess in megabytes
	ThreadMemMb  int    // if not zero then memory required for each thread in megabytes
}

// run job control file info
type runJobFile struct {
	filePath string // job control file path
	isError  bool   // if true then ignore that file due to error
	oms      string // oms instance name
	RunJob          // job control file content
}

// queue job control file info
type queueJobFile struct {
	runJobFile
	position int  // part of file name: queue position
	isPaused bool // if true then queue is paused
	isFirst  bool // if true then it is the first job in the queue
}

// job control file info for history job: parts of file name
type historyJobFile struct {
	filePath    string // job control file path
	isError     bool   // if true then ignore that file due to error
	SubmitStamp string // submission timestamp
	ModelName   string // model name
	ModelDigest string // model digest
	RunStamp    string // run stamp, if empty then auto-generated as timestamp
	JobStatus   string // run status
	RunTitle    string // model run title: run name, task run name or workset name
}

// RunState is model run state.
// Model run console output redirected to log file: modelName.YYYY_MM_DD_hh_mm_ss_SSS.console.log
type RunState struct {
	ModelName      string    // model name
	ModelDigest    string    // model digest
	RunStamp       string    // model run stamp, may be auto-generated as timestamp
	SubmitStamp    string    // submission timestamp
	IsFinal        bool      // final state, model completed
	UpdateDateTime string    // last update date-time
	RunName        string    // if not empty then run name
	TaskRunName    string    // if not empty then task run name
	IsLog          bool      // if true then use run log file
	LogFileName    string    // log file name
	logPath        string    // log file path: log/dir/modelName.RunStamp.console.log
	pid            int       // process id
	cmdPath        string    // executable path
	killC          chan bool // channel to kill model process
	isKill         bool      // if true then process killed
}

// runStateLog is model run state and log file lines.
type runStateLog struct {
	RunState            // model run state
	logUsedTs  int64    // unix seconds, last time when log file lines used
	logLineLst []string // model run log lines
}

// RunStateLogPage is run model status and page of the log lines.
type RunStateLogPage struct {
	RunState           // model run state
	Offset    int      // log page start line
	Size      int      // log page size
	TotalSize int      // log total run line count
	Lines     []string // page of log lines
}

// JobServiceState is a service state and job control state, it should NOT have any reference types members
type JobServiceState struct {
	IsQueuePaused     bool       // this oms instance: if true then jobs queue is paused, jobs are not selected from queue
	IsAllQueuePaused  bool       // all oms instances: if true then jobs queue is paused, jobs are not selected from queue
	JobUpdateDateTime string     // last date-time jobs list updated
	MpiRes            ComputeRes // MPI total available resources available (CPU cores and memory) as sum of all servers or localhost resources
	MaxOwnMpiRes      ComputeRes // resources limit (CPU cores and memory) for each oms instance
	ActiveTotalRes    ComputeRes // MPI active run resources (CPU cores and memory) used by all oms instances
	ActiveOwnRes      ComputeRes // MPI active run resources (CPU cores and memory) used by this oms instance
	QueueTotalRes     ComputeRes // MPI queue run resources (CPU cores and memory) requested by all oms instances
	QueueOwnRes       ComputeRes // MPI queue run resources (CPU cores and memory) requested by this oms instance
	MpiErrorRes       ComputeRes // MPI computational resources on "error" servers
	MpiMaxThreads     int        // max number of modelling threads per MPI process, zero means unlimited
	LocalRes          ComputeRes // localhost non-MPI jobs total resources limits
	LocalActiveRes    ComputeRes // localhost non-MPI jobs resources used by this instance to run models
	LocalQueueRes     ComputeRes // localhost non-MPI jobs queue resources for this oms instance
	isLeader          bool       // if true then this oms instance is a leader
	maxStartTime      int64      // max time in milliseconds to start compute server or cluster
	maxStopTime       int64      // max time in milliseconds to stop compute server or cluster
	maxIdleTime       int64      // max idle in milliseconds time before stopping server or cluster
	lastStartStopTs   int64      // last time when start or stop of computational servers done
	maxComputeErrors  int        // errors threshold for compute server or cluster
	jobLastPosition   int        // last job position in the queue
	jobFirstPosition  int        // minimal job position in the queue
	hostFile          hostIni    // MPI jobs hostfile settings
}

// computational server or cluster state
type computeItem struct {
	name       string     // name of server or cluster
	state      string     // state: start, stop, ready, error, empty "" means power off
	totalRes   ComputeRes // total computational resources (CPU cores and memory)
	usedRes    ComputeRes // resources (CPU cores and memory) used by all oms instances
	ownRes     ComputeRes // resources (CPU cores and memory) used by this instance
	errorCount int        // number of incomplete starts, stops and errors
	lastUsedTs int64      // last activity time (unix milliseconds): server start, stop or used
	startExe   string     // name of executable to start server, e.g.: /bin/sh
	startArgs  []string   // arguments to start server, e.g.: -c start.sh my-server-name
	stopExe    string     // name of executable to stop server,, e.g.: /bin/sh
	stopArgs   []string   // arguments to stop server, e.g.: -c stop.sh my-server-name
}

// computational server or cluster usage
type computeUse struct {
	name       string // name of server or cluster
	ComputeRes        // used computational resources (CPU cores and memory)
	filePath   string // if not empty then compute use file path
}

// job resources allocation by computational servers or clusters
type jobHostUse struct {
	oms     string       // oms instance name
	stamp   string       // submission timestamp
	res     RunRes       // actual run resources
	hostUse []computeUse // MPI job host usage
}

// Hostfile config from job.ini file: template to create hostfile for MPI model run
type hostIni struct {
	isUse    bool   // if true then create and use hostfile to run MPI jobs
	dir      string // HostFileDir = models/log
	hostName string // HostName = @-HOST-@
	cpuCores string // CpuCores = @-CORES-@
	rootLine string // RootLine = cpm slots=1 max_slots=1
	hostLine string // HostLine = @-HOST-@ slots=@-CORES-@
}

// job control state
type jobControlState struct {
	Queue []string // jobs queue
}

// timeout in msec, wait on stdout and stderr polling.
const logTickTimeout = 7

// file name of MPI model run template by default
const defaultMpiTemplate = "mpi.ModelRun.template.txt"

// RefreshCatalog reset state of most recent model run for each model.
func (rsc *RunCatalog) refreshCatalog(etcDir string, jsc *jobControlState) error {

	// make all models basic info: name, digest and files location
	mbs := theCatalog.allModels()
	rbs := make(map[string]modelRunBasic, len(mbs))

	for idx := range mbs {
		rbs[mbs[idx].model.Digest] = modelRunBasic{
			name:     mbs[idx].model.Name,
			binDir:   mbs[idx].binDir,
			logDir:   mbs[idx].logDir,
			isLogDir: mbs[idx].isLogDir,
		}
	}

	// read templates and run presets from etc directory
	rsc.updateEtcModelConfig(etcDir)

	// lock and update run state catalog
	rsc.rscLock.Lock()
	defer rsc.rscLock.Unlock()

	// model log history: add new models and delete existing models
	if rsc.modelRuns == nil {
		rsc.modelRuns = map[string]map[string]*runStateLog{}
	}
	// if model deleted then delete model logs history
	for d := range rsc.modelRuns {
		if _, ok := rbs[d]; !ok {
			delete(rsc.modelRuns, d)
		}
	}
	// if new model added then add new empty logs history
	for d := range rbs {
		if _, ok := rsc.modelRuns[d]; !ok {
			rsc.modelRuns[d] = map[string]*runStateLog{}
		}
	}
	rsc.models = rbs

	// cleanup jobs control state
	rsc.IsQueuePaused = true // pause jobs queue until jobs state updated current files

	rsc.JobUpdateDateTime = helper.MakeDateTime(time.Now())
	rsc.queueKeys = []string{}
	rsc.activeJobs = map[string]runJobFile{}
	rsc.queueJobs = map[string]queueJobFile{}
	rsc.historyJobs = make(map[string]historyJobFile, 1024) // assume long history of model runs
	rsc.computeState = map[string]computeItem{}
	rsc.cfgRes = map[string]modelCfgRes{}
	rsc.jobLastPosition = jobPositionDefault + 1
	rsc.jobFirstPosition = jobPositionDefault - 1
	if rsc.maxComputeErrors <= 1 {
		rsc.maxComputeErrors = maxComputeErrorsDefault
	}
	rsc.first = jobHostUse{hostUse: []computeUse{}}

	if rsc.selectedKeys == nil {
		rsc.selectedKeys = []string{}
	}
	if rsc.startupNames == nil {
		rsc.startupNames = []string{}
	}
	if rsc.shutdownNames == nil {
		rsc.shutdownNames = []string{}
	}

	if jsc != nil {
		if len(jsc.Queue) > 0 {
			rsc.queueKeys = append(rsc.queueKeys, jsc.Queue...)
		}
	}

	return nil
}

// read from etc directory model run template files and  run options preset files
func (rsc *RunCatalog) updateEtcModelConfig(etcDir string) {

	if !helper.IsDirExist(etcDir) {
		return
	}

	// get list of template files
	runTmpls := []string{}
	mpiTmpls := []string{}

	if fl, err := filepath.Glob(etcDir + "/" + "run.*.template.txt"); err == nil {
		for k := range fl {
			f := filepath.Base(fl[k])
			if f != "." && f != ".." && f != "/" && f != "\\" {
				runTmpls = append(runTmpls, f)
			}
		}
	}
	if fl, err := filepath.Glob(etcDir + "/" + "mpi.*.template.txt"); err == nil {
		for k := range fl {
			f := filepath.Base(fl[k])
			if f != "." && f != ".." && f != "/" && f != "\\" {
				mpiTmpls = append(mpiTmpls, f)
			}
		}
	}

	// read all run options preset files
	// keep stem of preset file name: run-options.RiskPaths.1-small.json => RiskPaths.1-small
	// and file content as string
	presets := []RunOptionsPreset{}

	if fl, err := filepath.Glob(etcDir + "/" + "run-options.*.json"); err == nil {
		for k := range fl {

			f := filepath.Base(fl[k])
			if len(f) < len("run-options.*.json") { // file name must be at least that size
				continue
			}
			bt, err := os.ReadFile(fl[k]) // read entire file
			if err != nil {
				continue // skip on errors
			}

			presets = append(presets,
				RunOptionsPreset{
					Name:    f[len("run-options.") : len(f)-(len(".json"))], // stem of the file: skip prefix and suffix
					Options: string(bt),
				})
		}
	}

	// lock and update run state catalog
	rsc.rscLock.Lock()
	defer rsc.rscLock.Unlock()

	// update etc directory and list of templates
	rsc.etcDir = etcDir
	rsc.runTemplates = runTmpls
	rsc.mpiTemplates = mpiTmpls
	rsc.presets = presets
}

// get "public" configuration of model run catalog
func (rsc *RunCatalog) toPublicConfig() *RunCatalogConfig {

	// lock run catalog and return results
	rsc.rscLock.Lock()
	defer rsc.rscLock.Unlock()

	rcp := RunCatalogConfig{
		RunTemplates:       make([]string, len(rsc.runTemplates)),
		DefaultMpiTemplate: defaultMpiTemplate,
		MpiTemplates:       make([]string, len(rsc.mpiTemplates)),
		Presets:            make([]RunOptionsPreset, len(rsc.presets)),
	}
	copy(rcp.RunTemplates, rsc.runTemplates)
	copy(rcp.MpiTemplates, rsc.mpiTemplates)
	copy(rcp.Presets, rsc.presets)

	return &rcp
}

// allModels return basic info from catalog about all models.
func (rsc *RunCatalog) allModels() map[string]modelRunBasic {
	rsc.rscLock.Lock()
	defer rsc.rscLock.Unlock()

	rbs := make(map[string]modelRunBasic, len(rsc.models))
	for key, val := range rsc.models {
		rbs[key] = val
	}
	return rbs
}

// return computational resources requirements for model run.
func (rsc *RunCatalog) getCfgRes(digest string) modelCfgRes {
	rsc.rscLock.Lock()
	defer rsc.rscLock.Unlock()

	if mr, ok := rsc.cfgRes[digest]; ok {
		return mr
	}
	return modelCfgRes{}
}
