// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"container/list"
	"database/sql"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"go.openmpp.org/ompp/config"
	"go.openmpp.org/ompp/db"
	"go.openmpp.org/ompp/helper"
	"go.openmpp.org/ompp/omppLog"
)

// copy model from database into text json and csv files
func dbToText(modelName string, modelDigest string, runOpts *config.RunOptions) error {

	// open source database connection and check is it valid
	cs, dn := db.IfEmptyMakeDefault(modelName, runOpts.String(config.DbConnectionStr), runOpts.String(config.DbDriverName))
	srcDb, _, err := db.Open(cs, dn, false)
	if err != nil {
		return err
	}
	defer srcDb.Close()

	nv, err := db.OpenmppSchemaVersion(srcDb)
	if err != nil || nv < db.MinSchemaVersion {
		return errors.New("invalid database, likely not an openM++ database")
	}

	// get model metadata
	modelDef, err := db.GetModel(srcDb, modelName, modelDigest)
	if err != nil {
		return err
	}
	modelName = modelDef.Model.Name // set model name: it can be empty and only model digest specified

	// create new output directory, use modelName subdirectory
	outDir := filepath.Join(runOpts.String(outputDirArgKey), modelName)
	err = os.MkdirAll(outDir, 0750)
	if err != nil {
		return err
	}

	// write model definition to json file
	if err = toModelJsonFile(srcDb, modelDef, outDir); err != nil {
		return err
	}

	// write all model run data into csv files: parameters, output expressions and accumulators
	dblFmt := runOpts.String(config.DoubleFormat)
	if err = toRunTextFileList(srcDb, modelDef, outDir, dblFmt); err != nil {
		return err
	}

	// write all readonly workset data into csv files: input parameters
	if err = toWorksetTextFileList(srcDb, modelDef, outDir, dblFmt); err != nil {
		return err
	}

	// write all modeling tasks and task run history to json files
	if err = toTaskJsonFileList(srcDb, modelDef, outDir); err != nil {
		return err
	}

	// pack model metadata, run results and worksets into zip
	if runOpts.Bool(zipArgKey) {
		zipPath, err := helper.PackZip(outDir, "")
		if err != nil {
			return err
		}
		omppLog.Log("Packed ", zipPath)
	}

	return nil
}

// copy model run from database into text json and csv files
func dbToTextRun(modelName string, modelDigest string, runOpts *config.RunOptions) error {

	// get model run name and id
	runName := runOpts.String(config.RunName)
	runId := runOpts.Int(config.RunId, 0)

	// conflicting options: use run id if positive else use run name
	if runOpts.IsExist(config.RunName) && runOpts.IsExist(config.RunId) {
		if runId > 0 {
			omppLog.Log("dbcopy options conflict. Using run id: ", runId, " ignore run name: ", runName)
			runName = ""
		} else {
			omppLog.Log("dbcopy options conflict. Using run name: ", runName, " ignore run id: ", runId)
			runId = 0
		}
	}

	if runId < 0 || runId == 0 && runName == "" {
		return errors.New("dbcopy invalid argument(s) for run id: " + runOpts.String(config.RunId) + " and/or run name: " + runOpts.String(config.RunName))
	}

	// open source database connection and check is it valid
	cs, dn := db.IfEmptyMakeDefault(modelName, runOpts.String(config.DbConnectionStr), runOpts.String(config.DbDriverName))
	srcDb, _, err := db.Open(cs, dn, false)
	if err != nil {
		return err
	}
	defer srcDb.Close()

	nv, err := db.OpenmppSchemaVersion(srcDb)
	if err != nil || nv < db.MinSchemaVersion {
		return errors.New("invalid database, likely not an openM++ database")
	}

	// get model metadata
	modelDef, err := db.GetModel(srcDb, modelName, modelDigest)
	if err != nil {
		return err
	}
	modelName = modelDef.Model.Name // set model name: it can be empty and only model digest specified

	// get model run metadata by id or name
	var runRow *db.RunRow
	var outDir string
	if runId > 0 {
		if runRow, err = db.GetRun(srcDb, runId); err != nil {
			return err
		}
		if runRow == nil {
			return errors.New("model run not found, id: " + strconv.Itoa(runId))
		}
		outDir = filepath.Join(runOpts.String(outputDirArgKey), modelName+".run."+strconv.Itoa(runId))
	} else {
		if runRow, err = db.GetRunByName(srcDb, modelDef.Model.ModelId, runName); err != nil {
			return err
		}
		if runRow == nil {
			return errors.New("model run not found: " + runName)
		}
		outDir = filepath.Join(runOpts.String(outputDirArgKey), modelName+".run."+runName)
	}

	// run must be completed: status success, error or exit
	if runRow.Status != db.DoneRunStatus && runRow.Status != db.ExitRunStatus && runRow.Status != db.ErrorRunStatus {
		return errors.New("model run not completed: " + strconv.Itoa(runRow.RunId) + " " + runRow.Name)
	}

	// get full model run metadata
	meta, err := db.GetRunFull(srcDb, runRow, "")
	if err != nil {
		return err
	}

	// create new "root" output directory for model run metadata
	// for csv files this "root" combined as root/run.1234.runName
	err = os.MkdirAll(outDir, 0750)
	if err != nil {
		return err
	}

	// write model run metadata into json, parameters and output result values into csv files
	dblFmt := runOpts.String(config.DoubleFormat)
	if err = toRunTextFile(srcDb, modelDef, meta, outDir, dblFmt); err != nil {
		return err
	}

	// pack model run metadata and results into zip
	if runOpts.Bool(zipArgKey) {
		zipPath, err := helper.PackZip(outDir, "")
		if err != nil {
			return err
		}
		omppLog.Log("Packed ", zipPath)
	}

	return nil
}

// copy workset from database into text json and csv files
func dbToTextWorkset(modelName string, modelDigest string, runOpts *config.RunOptions) error {

	// get workset name and id
	setName := runOpts.String(config.SetName)
	setId := runOpts.Int(config.SetId, 0)

	// conflicting options: use set id if positive else use set name
	if runOpts.IsExist(config.SetName) && runOpts.IsExist(config.SetId) {
		if setId > 0 {
			omppLog.Log("dbcopy options conflict. Using set id: ", setId, " ignore set name: ", setName)
			setName = ""
		} else {
			omppLog.Log("dbcopy options conflict. Using set name: ", setName, " ignore set id: ", setId)
			setId = 0
		}
	}

	if setId < 0 || setId == 0 && setName == "" {
		return errors.New("dbcopy invalid argument(s) for set id: " + runOpts.String(config.SetId) + " and/or set name: " + runOpts.String(config.SetName))
	}

	// open source database connection and check is it valid
	cs, dn := db.IfEmptyMakeDefault(modelName, runOpts.String(config.DbConnectionStr), runOpts.String(config.DbDriverName))
	srcDb, _, err := db.Open(cs, dn, false)
	if err != nil {
		return err
	}
	defer srcDb.Close()

	nv, err := db.OpenmppSchemaVersion(srcDb)
	if err != nil || nv < db.MinSchemaVersion {
		return errors.New("invalid database, likely not an openM++ database")
	}

	// get model metadata
	modelDef, err := db.GetModel(srcDb, modelName, modelDigest)
	if err != nil {
		return err
	}
	modelName = modelDef.Model.Name // set model name: it can be empty and only model digest specified

	// "root" directory for workset metadata
	// later this "root" combined with modelName.set.name or modelName.set.id
	outDir := ""
	if runOpts.IsExist(config.ParamDir) {
		outDir = filepath.Clean(runOpts.String(config.ParamDir))
	} else {
		outDir = runOpts.String(outputDirArgKey)
	}

	// get workset metadata by id or name
	var wsRow *db.WorksetRow
	if setId > 0 {
		if wsRow, err = db.GetWorkset(srcDb, setId); err != nil {
			return err
		}
		if wsRow == nil {
			return errors.New("workset not found, set id: " + strconv.Itoa(setId))
		}
		outDir = filepath.Join(outDir, modelName+".set."+strconv.Itoa(setId))
	} else {
		if wsRow, err = db.GetWorksetByName(srcDb, modelDef.Model.ModelId, setName); err != nil {
			return err
		}
		if wsRow == nil {
			return errors.New("workset not found: " + setName)
		}
		outDir = filepath.Join(outDir, modelName+".set."+setName)
	}

	wm, err := db.GetWorksetFull(srcDb, wsRow, "") // get full workset metadata
	if err != nil {
		return err
	}

	// check: workset must be readonly
	if !wm.Set.IsReadonly {
		return errors.New("workset must be readonly: " + strconv.Itoa(wsRow.SetId) + " " + wsRow.Name)
	}

	// create new output directory for workset metadata
	err = os.MkdirAll(outDir, 0750)
	if err != nil {
		return err
	}

	// write workset metadata into json and parameter values into csv files
	dblFmt := runOpts.String(config.DoubleFormat)
	if err = toWorksetTextFile(srcDb, modelDef, wm, outDir, dblFmt); err != nil {
		return err
	}

	// pack worksets metadata json and csv files into zip
	if runOpts.Bool(zipArgKey) {
		zipPath, err := helper.PackZip(outDir, "")
		if err != nil {
			return err
		}
		omppLog.Log("Packed ", zipPath)
	}

	return nil
}

// copy modeling task metadata and run history from database into text json and csv files
func dbToTextTask(modelName string, modelDigest string, runOpts *config.RunOptions) error {

	// get task name and id
	taskName := runOpts.String(config.TaskName)
	taskId := runOpts.Int(config.TaskId, 0)

	// conflicting options: use task id if positive else use task name
	if runOpts.IsExist(config.TaskName) && runOpts.IsExist(config.TaskId) {
		if taskId > 0 {
			omppLog.Log("dbcopy options conflict. Using task id: ", taskId, " ignore task name: ", taskName)
			taskName = ""
		} else {
			omppLog.Log("dbcopy options conflict. Using task name: ", taskName, " ignore task id: ", taskId)
			taskId = 0
		}
	}

	if taskId < 0 || taskId == 0 && taskName == "" {
		return errors.New("dbcopy invalid argument(s) for task id: " + runOpts.String(config.TaskId) + " and/or task name: " + runOpts.String(config.TaskName))
	}

	// open source database connection and check is it valid
	cs, dn := db.IfEmptyMakeDefault(modelName, runOpts.String(config.DbConnectionStr), runOpts.String(config.DbDriverName))
	srcDb, _, err := db.Open(cs, dn, false)
	if err != nil {
		return err
	}
	defer srcDb.Close()

	nv, err := db.OpenmppSchemaVersion(srcDb)
	if err != nil || nv < db.MinSchemaVersion {
		return errors.New("invalid database, likely not an openM++ database")
	}

	// get model metadata
	modelDef, err := db.GetModel(srcDb, modelName, modelDigest)
	if err != nil {
		return err
	}
	modelName = modelDef.Model.Name // set model name: it can be empty and only model digest specified

	// get task metadata by id or name
	var taskRow *db.TaskRow
	var outDir string
	if taskId > 0 {
		if taskRow, err = db.GetTask(srcDb, taskId); err != nil {
			return err
		}
		if taskRow == nil {
			return errors.New("modeling task not found, task id: " + strconv.Itoa(taskId))
		}
		outDir = filepath.Join(runOpts.String(outputDirArgKey), modelName+".task."+strconv.Itoa(taskId))
	} else {
		if taskRow, err = db.GetTaskByName(srcDb, modelDef.Model.ModelId, taskName); err != nil {
			return err
		}
		if taskRow == nil {
			return errors.New("modeling task not found: " + taskName)
		}
		outDir = filepath.Join(runOpts.String(outputDirArgKey), modelName+".task."+taskName)
	}

	meta, err := db.GetTaskFull(srcDb, taskRow, "") // get task full metadata, including task run history
	if err != nil {
		return err
	}

	// create new output directory for task metadata
	err = os.MkdirAll(outDir, 0750)
	if err != nil {
		return err
	}

	// write task metadata into json file
	if err = toTaskJsonFile(srcDb, modelDef, meta, outDir); err != nil {
		return err
	}

	// save runs from model run history
	var runIdLst []int
	var isRunNotFound, isRunNotCompleted bool
	dblFmt := runOpts.String(config.DoubleFormat)

	for j := range meta.TaskRun {
	nextRun:
		for k := range meta.TaskRun[j].TaskRunSet {

			// check is this run id already processed
			runId := meta.TaskRun[j].TaskRunSet[k].RunId
			for i := range runIdLst {
				if runId == runIdLst[i] {
					continue nextRun
				}
			}
			runIdLst = append(runIdLst, runId)

			// find model run metadata by id
			runRow, err := db.GetRun(srcDb, runId)
			if err != nil {
				return err
			}
			if runRow == nil {
				isRunNotFound = true
				continue // skip: run not found
			}

			// run must be completed: status success, error or exit
			if runRow.Status != db.DoneRunStatus && runRow.Status != db.ExitRunStatus && runRow.Status != db.ErrorRunStatus {
				isRunNotCompleted = true
				continue // skip: run not completed
			}

			rm, err := db.GetRunFull(srcDb, runRow, "") // get full model run metadata
			if err != nil {
				return err
			}

			// write model run metadata into json, parameters and output result values into csv files
			if err = toRunTextFile(srcDb, modelDef, rm, outDir, dblFmt); err != nil {
				return err
			}
		}
	}

	// find workset by set id and save it's metadata to json and workset parameters to csv
	var wsIdLst []int
	var isSetNotFound, isSetNotReadOnly bool

	var fws = func(dbConn *sql.DB, setId int) error {

		// check is workset already processed
		for i := range wsIdLst {
			if setId == wsIdLst[i] {
				return nil
			}
		}
		wsIdLst = append(wsIdLst, setId)

		// get workset by id
		wsRow, err := db.GetWorkset(dbConn, setId)
		if err != nil {
			return err
		}
		if wsRow == nil { // exit: workset not found
			isSetNotFound = true
			return nil
		}
		if !wsRow.IsReadonly { // exit: workset not readonly
			isSetNotReadOnly = true
			return nil
		}

		wm, err := db.GetWorksetFull(dbConn, wsRow, "") // get full workset metadata
		if err != nil {
			return err
		}

		// write workset metadata into json and parameter values into csv files
		if err = toWorksetTextFile(dbConn, modelDef, wm, outDir, dblFmt); err != nil {
			return err
		}
		return nil
	}

	// save task body worksets
	for k := range meta.Set {
		if err = fws(srcDb, meta.Set[k]); err != nil {
			return err
		}
	}

	// save worksets from model run history
	for j := range meta.TaskRun {
		for k := range meta.TaskRun[j].TaskRunSet {
			if err = fws(srcDb, meta.TaskRun[j].TaskRunSet[k].SetId); err != nil {
				return err
			}
		}
	}

	// display warnings if any worksets not found or not readonly
	// display warnings if any model runs not exists or not completed
	if isSetNotFound {
		omppLog.Log("Warning: task ", meta.Task.Name, " workset(s) not found, copy of task incomplete")
	}
	if isSetNotReadOnly {
		omppLog.Log("Warning: task ", meta.Task.Name, " workset(s) not readonly, copy of task incomplete")
	}
	if isRunNotFound {
		omppLog.Log("Warning: task ", meta.Task.Name, " model run(s) not found, copy of task run history incomplete")
	}
	if isRunNotCompleted {
		omppLog.Log("Warning: task ", meta.Task.Name, " model run(s) not completed, copy of task run history incomplete")
	}

	// pack worksets metadata json and csv files into zip
	if runOpts.Bool(zipArgKey) {
		zipPath, err := helper.PackZip(outDir, "")
		if err != nil {
			return err
		}
		omppLog.Log("Packed ", zipPath)
	}
	return nil
}

// toModelJsonFile convert model metadata to json and write into json files.
func toModelJsonFile(dbConn *sql.DB, modelDef *db.ModelMeta, outDir string) error {

	// get list of languages
	langDef, err := db.GetLanguages(dbConn)
	if err != nil {
		return err
	}

	// get model text (description and notes) in all languages
	modelTxt, err := db.GetModelText(dbConn, modelDef.Model.ModelId, "")
	if err != nil {
		return err
	}

	// get model parameter and output table groups (description and notes) in all languages
	modelGroup, err := db.GetModelGroup(dbConn, modelDef.Model.ModelId, "")
	if err != nil {
		return err
	}

	// get model profile: default model profile is profile where name = model name
	modelName := modelDef.Model.Name
	modelProfile, err := db.GetProfile(dbConn, modelName)
	if err != nil {
		return err
	}

	// save into model json files
	if err := helper.ToJsonFile(filepath.Join(outDir, modelName+".model.json"), &modelDef); err != nil {
		return err
	}
	if err := helper.ToJsonFile(filepath.Join(outDir, modelName+".lang.json"), &langDef); err != nil {
		return err
	}
	if err := helper.ToJsonFile(filepath.Join(outDir, modelName+".text.json"), &modelTxt); err != nil {
		return err
	}
	if err := helper.ToJsonFile(filepath.Join(outDir, modelName+".group.json"), &modelGroup); err != nil {
		return err
	}
	if err := helper.ToJsonFile(filepath.Join(outDir, modelName+".profile.json"), &modelProfile); err != nil {
		return err
	}
	return nil
}

// toRunTextFileList write all model runs parameters and output tables into csv files, each run in separate subdirectory
func toRunTextFileList(dbConn *sql.DB, modelDef *db.ModelMeta, outDir string, doubleFmt string) error {

	// get all successfully completed model runs
	rl, err := db.GetRunFullList(dbConn, modelDef.Model.ModelId, true, "")
	if err != nil {
		return err
	}

	// read all run parameters, output accumulators and expressions and dump it into csv files
	for k := range rl {
		err = toRunTextFile(dbConn, modelDef, &rl[k], outDir, doubleFmt)
		if err != nil {
			return err
		}
	}
	return nil
}

// toRunTextFile write model run metadata, parameters and output tables into csv files, in separate subdirectory
func toRunTextFile(dbConn *sql.DB, modelDef *db.ModelMeta, meta *db.RunMeta, outDir string, doubleFmt string) error {

	// convert db rows into "public" format
	runId := meta.Run.RunId
	omppLog.Log("Model run ", runId, " ", meta.Run.Name)

	pub, err := meta.ToPublic(dbConn, modelDef)
	if err != nil {
		return err
	}

	// create run subdir under model dir
	csvName := "run." + strconv.Itoa(runId) + "." + helper.ToAlphaNumeric(pub.Name)
	csvDir := filepath.Join(outDir, csvName)

	err = os.MkdirAll(csvDir, 0750)
	if err != nil {
		return err
	}

	layout := &db.ReadLayout{FromId: runId}

	// write all parameters into csv file
	for j := range modelDef.Param {

		layout.Name = modelDef.Param[j].Name

		cLst, err := db.ReadParameter(dbConn, modelDef, layout)
		if err != nil {
			return err
		}
		if cLst.Len() <= 0 { // parameter data must exist for all parameters
			return errors.New("missing run parameter values " + layout.Name + " run id: " + strconv.Itoa(layout.FromId))
		}

		var cp db.Cell
		err = toCsvFile(csvDir, modelDef, modelDef.Param[j].Name, cp, cLst, doubleFmt)
		if err != nil {
			return err
		}
	}

	// write all output tables into csv file
	for j := range modelDef.Table {

		// write output table expression values into csv file
		layout.Name = modelDef.Table[j].Name
		layout.IsAccum = false

		cLst, err := db.ReadOutputTable(dbConn, modelDef, layout)
		if err != nil {
			return err
		}

		var ec db.CellExpr
		err = toCsvFile(csvDir, modelDef, modelDef.Table[j].Name, ec, cLst, doubleFmt)
		if err != nil {
			return err
		}

		// write output table accumulators into csv file
		layout.IsAccum = true

		cLst, err = db.ReadOutputTable(dbConn, modelDef, layout)
		if err != nil {
			return err
		}

		var ac db.CellAcc
		err = toCsvFile(csvDir, modelDef, modelDef.Table[j].Name, ac, cLst, doubleFmt)
		if err != nil {
			return err
		}
	}

	// save model run metadata into json
	if err := helper.ToJsonFile(filepath.Join(outDir, modelDef.Model.Name+"."+csvName+".json"), pub); err != nil {
		return err
	}
	return nil
}

// toWorksetTextFileList write all readonly worksets into csv files, each set in separate subdirectory
func toWorksetTextFileList(dbConn *sql.DB, modelDef *db.ModelMeta, outDir string, doubleFmt string) error {

	// get all readonly worksets
	wl, err := db.GetWorksetFullList(dbConn, modelDef.Model.ModelId, true, "")
	if err != nil {
		return err
	}

	// read all workset parameters and dump it into csv files
	for k := range wl {
		err = toWorksetTextFile(dbConn, modelDef, &wl[k], outDir, doubleFmt)
		if err != nil {
			return err
		}
	}
	return nil
}

// toWorksetTextFile write workset into csv file, in separate subdirectory
func toWorksetTextFile(dbConn *sql.DB, modelDef *db.ModelMeta, meta *db.WorksetMeta, outDir string, doubleFmt string) error {

	// convert db rows into "public" format
	setId := meta.Set.SetId
	omppLog.Log("Workset ", setId, " ", meta.Set.Name)

	pub, err := meta.ToPublic(dbConn, modelDef)
	if err != nil {
		return err
	}

	// create workset subdir under output dir
	csvName := "set." + strconv.Itoa(setId) + "." + helper.ToAlphaNumeric(pub.Name)
	csvDir := filepath.Join(outDir, csvName)

	err = os.MkdirAll(csvDir, 0750)
	if err != nil {
		return err
	}

	layout := &db.ReadLayout{FromId: setId, IsFromSet: true}

	// write parameter into csv file
	for j := range pub.Param {

		layout.Name = pub.Param[j].Name

		cLst, err := db.ReadParameter(dbConn, modelDef, layout)
		if err != nil {
			return err
		}
		if cLst.Len() <= 0 { // parameter data must exist for all parameters
			return errors.New("missing workset parameter values " + layout.Name + " set id: " + strconv.Itoa(layout.FromId))
		}

		var cp db.Cell
		err = toCsvFile(csvDir, modelDef, modelDef.Param[j].Name, cp, cLst, doubleFmt)
		if err != nil {
			return err
		}
	}

	// save model workset metadata into json
	if err := helper.ToJsonFile(filepath.Join(outDir, modelDef.Model.Name+"."+csvName+".json"), pub); err != nil {
		return err
	}
	return nil
}

// toTaskJsonFileList convert all successfully completed tasks and tasks run history to json and write into json files
func toTaskJsonFileList(dbConn *sql.DB, modelDef *db.ModelMeta, outDir string) error {

	// get all modeling tasks and successfully completed tasks run history
	tl, err := db.GetTaskFullList(dbConn, modelDef.Model.ModelId, true, "")
	if err != nil {
		return err
	}

	// read each task metadata and write into json files
	for k := range tl {
		if err := toTaskJsonFile(dbConn, modelDef, &tl[k], outDir); err != nil {
			return err
		}
	}
	return nil
}

// toTaskJsonFile convert modeling task and task run history to json and write into json file
func toTaskJsonFile(dbConn *sql.DB, modelDef *db.ModelMeta, meta *db.TaskMeta, outDir string) error {

	// convert db rows into "public" format
	omppLog.Log("Modeling task ", meta.Task.TaskId, " ", meta.Task.Name)

	pub, err := meta.ToPublic(dbConn, modelDef)
	if err != nil {
		return err
	}

	// save modeling task metadata into json
	err = helper.ToJsonFile(filepath.Join(
		outDir,
		modelDef.Model.Name+".task."+strconv.Itoa(meta.Task.TaskId)+"."+helper.ToAlphaNumeric(meta.Task.Name)+".json"),
		pub)
	return err
}

// find workset by set id and save it's metadata to json and workset parameters to csv
func worksetToTextById(dbConn *sql.DB, modelDef *db.ModelMeta, setId int, outDir string, doubleFmt string) (bool, bool, error) {

	// get workset by id
	wsRow, err := db.GetWorkset(dbConn, setId)
	if err != nil {
		return false, false, err
	}
	if wsRow == nil { // exit: workset not found
		return true, false, nil
	}
	if !wsRow.IsReadonly { // exit: workset not readonly
		return false, true, nil
	}

	wm, err := db.GetWorksetFull(dbConn, wsRow, "") // get full workset metadata
	if err != nil {
		return false, false, err
	}

	// write workset metadata into json and parameter values into csv files
	if err = toWorksetTextFile(dbConn, modelDef, wm, outDir, doubleFmt); err != nil {
		return false, false, err
	}
	return false, false, nil
}

// toCsvFile convert parameter or output table values and write into csvDir/fileName.csv file.
func toCsvFile(
	csvDir string, modelDef *db.ModelMeta, name string, cell db.CsvConverter, cellLst *list.List, doubleFmt string) error {

	// converter from db cell to csv row []string
	cvt, err := cell.CsvToRow(modelDef, name, doubleFmt)
	if err != nil {
		return err
	}

	// create csv file
	fn, err := cell.CsvFileName(modelDef, name)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Join(csvDir, fn), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	wr := csv.NewWriter(f)

	// write header line: column names
	cs, err := cell.CsvHeader(modelDef, name, false)
	if err != nil {
		return err
	}
	if err = wr.Write(cs); err != nil {
		return err
	}

	for c := cellLst.Front(); c != nil; c = c.Next() {

		// write cell line: dimension(s) and value
		if err := cvt(c.Value, cs); err != nil {
			return err
		}
		if err := wr.Write(cs); err != nil {
			return err
		}
	}

	// flush and return error, if any
	wr.Flush()
	return wr.Error()
}
