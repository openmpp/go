// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/openmpp/go/ompp/db"
	"github.com/openmpp/go/ompp/helper"
	"github.com/openmpp/go/ompp/omppLog"
)

// runModel starts new model run and return run stamp.
// if run stamp not specified as input parameter then use unique timestamp.
// Model run console output redirected to log file: models/log/modelName.runStamp.console.log
func (rsc *RunCatalog) runModel(submitStamp string, req *RunRequest) (*RunState, error) {

	// make model process run stamp, if not specified then use timestamp by default
	ts, dtNow := theCatalog.getNewTimeStamp()
	rStamp := helper.CleanPath(req.RunStamp)
	if rStamp == "" {
		rStamp = ts
	}

	// new run state
	rs := &RunState{
		ModelName:      req.ModelName,
		ModelDigest:    req.ModelDigest,
		RunStamp:       rStamp,
		SubmitStamp:    submitStamp,
		UpdateDateTime: helper.MakeDateTime(dtNow),
	}

	// set directories: work directory and bin model.exe directory
	// if bin directory is relative then it must be relative to oms root directory
	// re-base it to model work directory
	binRoot, _ := theCatalog.getModelDir()

	mb, ok := theCatalog.modelBasicByDigest(rs.ModelDigest)
	if !ok {
		err := errors.New("Model not found: " + rs.ModelName + ": " + rs.ModelDigest)
		omppLog.Log("Model run error: ", err)
		moveJobQueueToFailed(rs.SubmitStamp, rs.ModelName, rs.ModelDigest)
		rs.IsFinal = true
		return rs, err // exit with error: model failed to start
	}
	binDir := mb.binDir

	wDir := binDir
	if req.Dir != "" {
		wDir = filepath.Join(binRoot, req.Dir)
	}

	binDir, err := filepath.Rel(wDir, binDir)
	if err != nil {
		binDir = binRoot
	}

	// make file path for model console log output
	if mb.isLogDir {
		if mb.logDir == "" {
			mb.logDir, mb.isLogDir = theCatalog.getModelLogDir()
		}
	}
	if mb.isLogDir {
		if mb.logDir == "" {
			mb.logDir = binDir
		}
		rs.IsLog = mb.isLogDir
		rs.LogFileName = rs.ModelName + "." + rStamp + ".console.log"
		rs.logPath = filepath.Join(mb.logDir, rs.LogFileName)
	}

	// make model run command line arguments, starting from process run stamp and log options
	mArgs := []string{}
	mArgs = append(mArgs, "-OpenM.RunStamp", rStamp)
	mArgs = append(mArgs, "-OpenM.LogToConsole", "true")
	mArgs = append(mArgs, "-OpenM.LogToFile", "false")

	importDbLc := strings.ToLower("-ImportDb.")

	// append model run options from run request
	for krq, val := range req.Opts {

		if len(krq) < 1 { // skip empty run options
			continue
		}

		// command line argument key starts with "-" ex: "-OpenM.Threads"
		key := krq
		if krq[0] != '-' {
			key = "-" + krq
		}

		// save run name and task run name to return as part of run state
		if strings.EqualFold(key, "-OpenM.RunName") {
			rs.RunName = val
		}
		if strings.EqualFold(key, "-OpenM.TaskRunName") {
			rs.TaskRunName = val
		}
		if strings.EqualFold(key, "-OpenM.LogToConsole") {
			continue // skip log to console input run option: it is already on
		}
		if strings.EqualFold(key, "-OpenM.LogToFile") {
			continue // skip log to file input run option: replaced by console output
		}
		if strings.EqualFold(key, "-OpenM.LogFilePath") {
			continue // skip log file path input run option: replaced by console output
		}
		if strings.EqualFold(key, "-OpenM.Database") {
			continue // database connection string not allowed as run option
		}
		if strings.HasPrefix(strings.ToLower(key), importDbLc) {
			continue // import database connection string not allowed as run option
		}

		mArgs = append(mArgs, key, val) // append command line argument key and value
	}

	// if list of tables to retain is not empty then put the list into ini-file:
	//
	//   [Tables]
	//   Retain   = ageSexIncome,AdditionalTables
	//
	if len(req.Tables) > 0 {
		p, e := filepath.Abs(filepath.Join(mb.logDir, rs.RunStamp+"."+mb.name+".ini"))
		if e == nil {
			e = os.WriteFile(p, []byte("[Tables]"+"\n"+"Retain = "+strings.Join(req.Tables, ", ")+"\n"), 0644)
		}
		if e != nil {
			omppLog.Log("Model run error: ", e)
			moveJobQueueToFailed(rs.SubmitStamp, rs.ModelName, rs.ModelDigest)
			rs.IsFinal = true
			return rs, e
		}
		mArgs = append(mArgs, "-ini", p) // append ini file path to command line arguments
	}

	// save run notes into the file(s) and append file path(s) to the model run options
	for _, rn := range req.RunNotes {
		if rn.Note == "" {
			continue
		}
		if !rs.IsLog {
			e := errors.New("Unable to save run notes: " + rs.ModelName + ": " + rs.ModelDigest)
			omppLog.Log("Model run error: ", e)
			moveJobQueueToFailed(rs.SubmitStamp, rs.ModelName, rs.ModelDigest)
			rs.IsFinal = true
			return rs, e
		}

		p, e := filepath.Abs(filepath.Join(mb.logDir, rs.RunStamp+".run_notes."+rn.LangCode+".md"))
		if e == nil {
			e = os.WriteFile(p, []byte(rn.Note), 0644)
		}
		if e != nil {
			omppLog.Log("Model run error: ", e)
			moveJobQueueToFailed(rs.SubmitStamp, rs.ModelName, rs.ModelDigest)
			rs.IsFinal = true
			return rs, e
		}

		mArgs = append(mArgs, "-"+rn.LangCode+".RunNotesPath", p) // append run notes file path to command line arguments
	}

	// assume model exe name is the same as model name
	mExe := helper.CleanPath(rs.ModelName)

	cmd, err := rsc.makeCommand(mExe, binDir, wDir, mb.dbPath, mArgs, req)
	if err != nil {
		omppLog.Log("Error at starting model: ", err)
		moveJobQueueToFailed(rs.SubmitStamp, rs.ModelName, rs.ModelDigest)
		rs.IsFinal = true
		return rs, errors.New("Error at starting model " + rs.ModelName + ": " + err.Error())
	}

	// connect console output to log line array
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		omppLog.Log("Error at starting model: ", err)
		moveJobQueueToFailed(rs.SubmitStamp, rs.ModelName, rs.ModelDigest)
		rs.IsFinal = true
		return rs, errors.New("Error at starting model " + rs.ModelName + ": " + err.Error())
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		omppLog.Log("Error at starting model: ", err)
		moveJobQueueToFailed(rs.SubmitStamp, rs.ModelName, rs.ModelDigest)
		rs.IsFinal = true
		return rs, errors.New("Error at starting model " + rs.ModelName + ": " + err.Error())
	}
	outDoneC := make(chan bool, 1)
	errDoneC := make(chan bool, 1)
	killC := make(chan bool, 1)
	logTck := time.NewTicker(logTickTimeout * time.Millisecond)

	// append console output to log lines array
	doLog := func(rState *RunState, r io.Reader, done chan<- bool) {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			rsc.updateRunStateLog(rState, false, sc.Text())
		}
		done <- true
		close(done)
	}

	// run state initialized: save in run state list
	// create model run log file
	rsc.createRunStateLog(rs)

	// start console output listners
	go doLog(rs, outPipe, outDoneC)
	go doLog(rs, errPipe, errDoneC)

	// start the model
	omppLog.Log("Run model: ", mExe, " in directory: ", wDir)
	omppLog.Log(strings.Join(cmd.Args, " "))
	rsc.updateRunStateProcess(rs, false, cmd.Path, 0, killC)

	err = cmd.Start()
	if err != nil {
		omppLog.Log("Model run error: ", err)
		moveJobQueueToFailed(rs.SubmitStamp, rs.ModelName, rs.ModelDigest)
		rsc.updateRunStateLog(rs, true, err.Error())
		rs.IsFinal = true
		return rs, err // exit with error: model failed to start
	}
	// else model started
	rsc.updateRunStateProcess(rs, false, cmd.Path, cmd.Process.Pid, killC)
	moveJobToActive(rs.SubmitStamp, rs.ModelName, rs.ModelDigest, rStamp, cmd.Process.Pid, cmd.Path)

	//  wait until run completed or terminated
	go func(rState *RunState, cmd *exec.Cmd) {

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
			case isKill, ok := <-killC:
				if !ok {
					killC = nil
				}
				if isKill && ok {
					omppLog.Log("Kill run: ", rState.ModelName, " ", rState.ModelDigest, " ", rState.RunName, " ", rState.RunStamp)
					if e := cmd.Process.Kill(); e != nil {
						omppLog.Log(e)
					}
				}
			case <-logTck.C:
			}
		}

		// wait for model run to be completed
		e := cmd.Wait()
		if e != nil {
			omppLog.Log("Model run error: ", e)
			rsc.updateRunStateLog(rState, true, e.Error())
			moveJobToHistory(db.ErrorRunStatus, rState.SubmitStamp, rState.ModelName, rState.ModelDigest, rState.RunStamp, cmd.Process.Pid)
			_, e = theCatalog.UpdateRunStatus(rState.ModelDigest, rState.RunStamp, db.ErrorRunStatus)
			if e != nil {
				omppLog.Log(e)
			}
			return
		}
		// else: completed OK
		rsc.updateRunStateLog(rState, true, "")
		moveJobToHistory(db.DoneRunStatus, rState.SubmitStamp, rState.ModelName, rState.ModelDigest, rState.RunStamp, cmd.Process.Pid)

	}(rs, cmd)

	return rs, nil
}

// makeCommand return command to run the model.
// If template file name specified then template processing results used to create command line.
// If this is MPI model run then tempalate is requred
// MPI run template can be model specific: "mpi.ModelName.template.txt" or default: "mpi.ModelRun.template.txt".
func (rsc *RunCatalog) makeCommand(mExe, binDir, workDir, dbPath string, mArgs []string, req *RunRequest) (*exec.Cmd, error) {

	// check is it MPI model run, to run MPI model template is required
	isMpi := req.Mpi.Np != 0
	if isMpi && req.Template == "" {

		// search for model-specific MPI template
		mtn := "mpi." + req.ModelName + ".template.txt"

		for _, tn := range theRunCatalog.mpiTemplates {
			if tn == mtn {
				req.Template = mtn
			}
		}
		// if model-specific MPI template not found then use default MPI template to run the model
		if req.Template == "" {
			req.Template = defaultMpiTemplate
		}
	}
	isTmpl := req.Template != ""

	// if this is regular non-MPI model.exe run and no template:
	//	 ./modelExe -OpenM.LogToFile true ...etc...
	var cmd *exec.Cmd

	if !isTmpl && !isMpi {
		if binDir == "" || binDir == "." || binDir == "./" {
			mExe = "./" + mExe
		} else {
			mExe = filepath.Join(binDir, mExe)
		}
		cmd = exec.Command(mExe, mArgs...)
	}

	// if template specified then process template to get exe name and arguments
	if isTmpl {

		// parse template
		tmpl, err := template.ParseFiles(filepath.Join(rsc.etcDir, req.Template))
		if err != nil {
			return nil, err
		}

		// set template parameters
		wd, err := filepath.Abs(workDir)
		if err != nil {
			return nil, err
		}
		d := struct {
			ModelName string            // model name
			ExeStem   string            // base part of model exe name, usually modelName
			Dir       string            // work directory to run the model
			BinDir    string            // bin directory where model.exe is located
			DbPath    string            // absolute path to sqlite database file: models/bin/model.sqlite
			MpiNp     int               // number of MPI processes
			Args      []string          // model command line arguments
			Env       map[string]string // environment variables to run the model
		}{
			ModelName: req.ModelName,
			ExeStem:   mExe,
			Dir:       wd,
			BinDir:    binDir,
			DbPath:    dbPath,
			MpiNp:     req.Mpi.Np,
			Args:      mArgs,
			Env:       req.Env,
		}

		// execute template and convert results in array of text lines
		var b strings.Builder

		err = tmpl.Execute(&b, d)
		if err != nil {
			return nil, err
		}
		tLines := strings.Split(strings.Replace(b.String(), "\r", "\n", -1), "\n")

		// from template processing results get:
		//   exe name as first non-empty line
		//   use all other non-empty lines as command line arguments
		cExe := ""
		cArgs := []string{}

		for k := range tLines {

			cl := strings.TrimSpace(tLines[k])
			if cl == "" {
				continue
			}
			if cExe == "" {
				cExe = cl
			} else {
				cArgs = append(cArgs, cl)
			}
		}
		if cExe == "" {
			return nil, errors.New("Error: empty template processing results, cannot run the model: " + req.ModelName)
		}

		// make command
		cmd = exec.Command(cExe, cArgs...)
	}

	// if this is not MPI run then:
	// 	set work directory
	// 	append request environment variables to model environment
	if !isMpi {

		cmd.Dir = workDir

		if len(req.Env) > 0 {
			env := os.Environ()
			for key, val := range req.Env {
				if key != "" && val != "" {
					env = append(env, key+"="+val)
				}
			}
			cmd.Env = env
		}
	}

	return cmd, nil
}
