// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"github.com/openmpp/go/ompp/db"
	"github.com/openmpp/go/ompp/omppLog"
	"golang.org/x/text/language"
)

// TaskList return list of task_lst db rows by model digest-or-name.
// No text info returned (no description and notes).
func (mc *ModelCatalog) TaskList(dn string) ([]db.TaskPub, bool) {

	// if model digest-or-name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return []db.TaskPub{}, false
	}

	// load model metadata in order to convert to "public"
	if _, ok := mc.loadModelMeta(dn); !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return []db.TaskPub{}, false // return empty result: model not found or error
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return []db.TaskPub{}, false // return empty result: model not found or error
	}

	tl, err := db.GetTaskList(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId)
	if err != nil {
		omppLog.Log("Error at get task list: ", dn, ": ", err.Error())
		return []db.TaskPub{}, false // return empty result: task select error
	}
	if len(tl) <= 0 {
		return []db.TaskPub{}, true // return empty result: task_lst rows not found for that model
	}

	// for each task_lst convert it to "public" task format
	tpl := make([]db.TaskPub, len(tl))

	for ni := range tl {

		p, err := (&db.TaskMeta{TaskDef: db.TaskDef{Task: tl[ni]}}).ToPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
		if err != nil {
			omppLog.Log("Error at task conversion: ", dn, ": ", err.Error())
			return []db.TaskPub{}, false // return empty result: conversion error
		}
		if p != nil {
			tpl[ni] = *p
		}
	}

	return tpl, true
}

// TaskListText return list of task_lst and task_txt db rows by model digest-or-name.
// Text (description and notes) are in preferred language or if text in such language exists.
func (mc *ModelCatalog) TaskListText(dn string, preferredLang []language.Tag) ([]db.TaskPub, bool) {

	// if model digest-or-name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return []db.TaskPub{}, false
	}

	// load model metadata in order to convert to "public"
	if _, ok := mc.loadModelMeta(dn); !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return []db.TaskPub{}, false // return empty result: model not found or error
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return []db.TaskPub{}, false // return empty result: model not found or error
	}

	// get task_txt db row for each task_lst using matched preferred language
	_, np, _ := mc.modelLst[idx].matcher.Match(preferredLang...)
	lc := mc.modelLst[idx].langCodes[np]

	tl, txl, err := db.GetTaskListText(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, lc)
	if err != nil {
		omppLog.Log("Error at get task list: ", dn, ": ", err.Error())
		return []db.TaskPub{}, false // return empty result: task select error
	}
	if len(tl) <= 0 {
		return []db.TaskPub{}, true // return empty result: task_lst rows not found for that model
	}

	// for each task_lst find task_txt row if exist and convert to "public" task format
	tpl := make([]db.TaskPub, len(tl))

	nt := 0
	for ni := range tl {

		// find text row for current master row by task id
		isFound := false
		for ; nt < len(txl); nt++ {
			isFound = txl[nt].TaskId == tl[ni].TaskId
			if txl[nt].TaskId >= tl[ni].TaskId {
				break // text found or text missing: text task id ahead of master task id
			}
		}

		// convert to "public" format
		var p *db.TaskPub
		var err error

		if isFound && nt < len(txl) {
			p, err = (&db.TaskMeta{TaskDef: db.TaskDef{Task: tl[ni], Txt: []db.TaskTxtRow{txl[nt]}}}).ToPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
		} else {
			p, err = (&db.TaskMeta{TaskDef: db.TaskDef{Task: tl[ni]}}).ToPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
		}
		if err != nil {
			omppLog.Log("Error at task conversion: ", dn, ": ", err.Error())
			return []db.TaskPub{}, false // return empty result: conversion error
		}
		if p != nil {
			tpl[ni] = *p
		}
	}

	return tpl, true
}

// TaskSets return task definition: task_lst master row and task sets by model digest-or-name and task name.
func (mc *ModelCatalog) TaskSets(dn, tn string) (*db.TaskPub, bool) {

	// if model digest-or-name or task name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return &db.TaskPub{}, false
	}
	if tn == "" {
		omppLog.Log("Warning: invalid (empty) task name")
		return &db.TaskPub{}, false
	}

	// load model metadata in order to convert to "public"
	if _, ok := mc.loadModelMeta(dn); !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.TaskPub{}, false // return empty result: model not found or error
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.TaskPub{}, false // return empty result: model not found or error
	}

	// find modeling task: get task_lst db row by task name
	tr, err := db.GetTaskByName(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, tn)
	if err != nil {
		omppLog.Log("Error at get modeling task: ", dn, ": ", tn, ": ", err.Error())
		return &db.TaskPub{}, false // return empty result: task select error
	}
	if tr == nil {
		omppLog.Log("Warning modeling task not found: ", dn, ": ", tn)
		return &db.TaskPub{}, false // return empty result: task_lst row not found
	}

	// get list of task set_id's from task_set
	setIds, err := db.GetTaskSetIds(mc.modelLst[idx].dbConn, tr.TaskId)
	if err != nil {
		omppLog.Log("Error at get modeling task list of input sets: ", dn, ": ", tn, ": ", err.Error())
		return &db.TaskPub{}, false // return empty result: task set id's select error
	}

	// convert to "public" modeling task format
	tp, err := (&db.TaskMeta{TaskDef: db.TaskDef{Task: *tr, Set: setIds}}).ToPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
	if err != nil {
		omppLog.Log("Error at modeling task conversion: ", dn, ": ", tn, ": ", err.Error())
		return &db.TaskPub{}, false // return empty result: error to convert task to pulic format
	}

	return tp, true
}

// TaskRuns return task run history by model digest-or-name and task name:
// task_lst master row and task run(s) details from task_run_lst and task_run_set tables.
func (mc *ModelCatalog) TaskRuns(dn, tn string) (*db.TaskPub, bool) {

	// if model digest-or-name or task name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return &db.TaskPub{}, false
	}
	if tn == "" {
		omppLog.Log("Warning: invalid (empty) task name")
		return &db.TaskPub{}, false
	}

	// load model metadata in order to convert to "public"
	if _, ok := mc.loadModelMeta(dn); !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.TaskPub{}, false // return empty result: model not found or error
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.TaskPub{}, false // return empty result: model not found or error
	}

	// find modeling task: get task_lst db row by task name
	tr, err := db.GetTaskByName(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, tn)
	if err != nil {
		omppLog.Log("Error at get modeling task: ", dn, ": ", tn, ": ", err.Error())
		return &db.TaskPub{}, false // return empty result: task select error
	}
	if tr == nil {
		omppLog.Log("Warning modeling task not found: ", dn, ": ", tn)
		return &db.TaskPub{}, false // return empty result: task_lst row not found
	}

	// get task run history
	tm, err := db.GetTaskRunList(mc.modelLst[idx].dbConn, tr)
	if err != nil {
		omppLog.Log("Error at get modeling task run history: ", dn, ": ", tn, ": ", err.Error())
		return &db.TaskPub{}, false // return empty result: select error
	}

	// convert to "public" modeling task format
	tp, err := tm.ToPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
	if err != nil {
		omppLog.Log("Error at modeling task conversion: ", dn, ": ", tn, ": ", err.Error())
		return &db.TaskPub{}, false // return empty result: error to convert task to pulic format
	}

	return tp, true
}

// TaskRunStatus return task_run_lst db row by model digest-or-name, task name and task run stamp or run name.
func (mc *ModelCatalog) TaskRunStatus(dn, tn, trsn string) (*db.TaskRunRow, bool) {

	// if model digest-or-name or task name or task run nam is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return &db.TaskRunRow{}, false
	}
	if tn == "" {
		omppLog.Log("Warning: invalid (empty) task name")
		return &db.TaskRunRow{}, false
	}
	if trsn == "" {
		omppLog.Log("Warning: invalid (empty) task run stamp or name")
		return &db.TaskRunRow{}, false
	}

	// load model metadata in order to convert to "public"
	if _, ok := mc.loadModelMeta(dn); !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.TaskRunRow{}, false // return empty result: model not found or error
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.TaskRunRow{}, false // return empty result: model not found or error
	}

	// find modeling task: get task_lst db row by task name
	tr, err := db.GetTaskByName(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, tn)
	if err != nil {
		omppLog.Log("Error at get modeling task: ", dn, ": ", tn, ": ", err.Error())
		return &db.TaskRunRow{}, false // return empty result: task select error
	}
	if tr == nil {
		omppLog.Log("Warning modeling task not found: ", dn, ": ", tn)
		return &db.TaskRunRow{}, false // return empty result: task_lst row not found
	}

	// get task run row by run stamp or run name and task id
	rst, err := db.GetTaskRunByStampOrName(mc.modelLst[idx].dbConn, tr.TaskId, trsn)
	if err != nil {
		omppLog.Log("Error at get modeling task run status: ", dn, ": ", tn, ": ", trsn, ": ", err.Error())
		return &db.TaskRunRow{}, false // return empty result: select error
	}
	if rst == nil {
		// omppLog.Log("Warning modeling task run not found: ", dn, ": ", tn, ": ", trsn)
		return &db.TaskRunRow{}, false // return empty result: task_lst row not found or not belong to the task
	}

	return rst, true
}

// TaskRunStatusList return list of task_run_lst db rows by model digest-or-name, task name and task run stamp or run name.
func (mc *ModelCatalog) TaskRunStatusList(dn, tn, trsn string) ([]db.TaskRunRow, bool) {

	// if model digest-or-name or task name or task run nam is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return []db.TaskRunRow{}, false
	}
	if tn == "" {
		omppLog.Log("Warning: invalid (empty) task name")
		return []db.TaskRunRow{}, false
	}
	if trsn == "" {
		omppLog.Log("Warning: invalid (empty) task run stamp or name")
		return []db.TaskRunRow{}, false
	}

	// load model metadata in order to convert to "public"
	if _, ok := mc.loadModelMeta(dn); !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return []db.TaskRunRow{}, false // return empty result: model not found or error
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return []db.TaskRunRow{}, false // return empty result: model not found or error
	}

	// find modeling task: get task_lst db row by task name
	tr, err := db.GetTaskByName(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, tn)
	if err != nil {
		omppLog.Log("Error at get modeling task: ", dn, ": ", tn, ": ", err.Error())
		return []db.TaskRunRow{}, false // return empty result: task select error
	}
	if tr == nil {
		omppLog.Log("Warning modeling task not found: ", dn, ": ", tn)
		return []db.TaskRunRow{}, false // return empty result: task_lst row not found
	}

	// get task run row by run stamp or run name and task id
	rLst, err := db.GetTaskRunListByStampOrName(mc.modelLst[idx].dbConn, tr.TaskId, trsn)
	if err != nil {
		omppLog.Log("Error at get modeling task run status: ", dn, ": ", tn, ": ", trsn, ": ", err.Error())
		return []db.TaskRunRow{}, false // return empty result: select error
	}
	if len(rLst) <= 0 {
		// omppLog.Log("Warning modeling task run not found: ", dn, ": ", tn, ": ", trsn)
		return []db.TaskRunRow{}, false // return empty result: task_lst row not found or not belong to the task
	}

	return rLst, true
}

// FirstOrLastTaskRunStatus return first or last task_run_lst db row by model digest-or-name and task name.
func (mc *ModelCatalog) FirstOrLastTaskRunStatus(dn, tn string, isFirst, isCompleted bool) (*db.TaskRunRow, bool) {

	// if model digest-or-name or task name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return &db.TaskRunRow{}, false
	}
	if tn == "" {
		omppLog.Log("Warning: invalid (empty) task name")
		return &db.TaskRunRow{}, false
	}

	// load model metadata in order to convert to "public"
	if _, ok := mc.loadModelMeta(dn); !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.TaskRunRow{}, false // return empty result: model not found or error
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.TaskRunRow{}, false // return empty result: model not found or error
	}

	// find modeling task: get task_lst db row by task name
	tr, err := db.GetTaskByName(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, tn)
	if err != nil {
		omppLog.Log("Error at get modeling task: ", dn, ": ", tn, ": ", err.Error())
		return &db.TaskRunRow{}, false // return empty result: task select error
	}
	if tr == nil {
		omppLog.Log("Warning modeling task not found: ", dn, ": ", tn)
		return &db.TaskRunRow{}, false // return empty result: task_lst row not found
	}

	// get first or last task run row
	rst := &db.TaskRunRow{}
	if isFirst {
		rst, err = db.GetTaskFirstRun(mc.modelLst[idx].dbConn, tr.TaskId)
	} else {
		if isCompleted {
			rst, err = db.GetTaskLastCompletedRun(mc.modelLst[idx].dbConn, tr.TaskId)
		} else {
			rst, err = db.GetTaskLastRun(mc.modelLst[idx].dbConn, tr.TaskId)
		}
	}
	if err != nil {
		omppLog.Log("Error at get modeling task run status: ", dn, ": ", tn, ": ", err.Error())
		return &db.TaskRunRow{}, false // return empty result: select error
	}
	if rst == nil {
		// omppLog.Log("Warning modeling task run not found: ", dn, ": ", tn)
		return &db.TaskRunRow{}, false // return empty result: task_lst row not found or not belong to the task
	}

	return rst, true
}

// TaskTextFull return full task metadata, description, notes, run history by model digest-or-name and task name
// from db-tables: task_lst, task_txt, task_set, task_run_lst, task_run_set.
// It does not return non-completed task runs (run in progress).
// Run completed if run status one of: s=success, x=exit, e=error.
// Text (description and notes) can be in preferred language or all languages.
// If preferred language requested and it is not found in db then return empty text results.
func (mc *ModelCatalog) TaskTextFull(dn, tn string, isAllLang bool, preferredLang []language.Tag) (*db.TaskPub, *db.TaskRunSetTxt, bool) {

	// if model digest-or-name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return &db.TaskPub{}, nil, false
	}

	// load model metadata in order to convert to "public"
	if _, ok := mc.loadModelMeta(dn); !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.TaskPub{}, nil, false // return empty result: model not found or error
	}

	// lock catalog and find model index by digest or name
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	idx, ok := mc.indexByDigestOrName(dn)
	if !ok {
		omppLog.Log("Warning: model digest or name not found: ", dn)
		return &db.TaskPub{}, nil, false // return empty result: model not found or error
	}

	// get task_lst db row by task name
	tr, err := db.GetTaskByName(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta.Model.ModelId, tn)
	if err != nil {
		omppLog.Log("Error at get modeling task: ", dn, ": ", tn, ": ", err.Error())
		return &db.TaskPub{}, nil, false // return empty result: task select error
	}
	if tr == nil {
		omppLog.Log("Warning modeling task not found: ", dn, ": ", tn)
		return &db.TaskPub{}, nil, false // return empty result: task_lst row not found
	}

	// get full metadata db rows using matched preferred language or in all languages
	lc := ""
	if !isAllLang {
		_, np, _ := mc.modelLst[idx].matcher.Match(preferredLang...)
		lc = mc.modelLst[idx].langCodes[np]
	}

	tm, err := db.GetTaskFull(mc.modelLst[idx].dbConn, tr, lc)
	if err != nil {
		omppLog.Log("Error at get modeling task text: ", dn, ": ", tr.Name, ": ", err.Error())
		return &db.TaskPub{}, nil, false // return empty result: run select error
	}

	// convert to "public" model run format
	tp, err := tm.ToPublic(mc.modelLst[idx].dbConn, mc.modelLst[idx].meta)
	if err != nil {
		omppLog.Log("Error at modeling task conversion: ", dn, ": ", tn, ": ", err.Error())
		return &db.TaskPub{}, nil, false // return empty result: conversion error
	}

	// get additinal task text: description and notes for worksets and model runs
	at, err := db.GetTaskRunSetText(mc.modelLst[idx].dbConn, tr.TaskId, lc)
	if err != nil {
		omppLog.Log("Error at get additional modeling task text: ", dn, ": ", tr.Name, ": ", err.Error())
		return &db.TaskPub{}, nil, false // return empty result: conversion error
	}

	return tp, at, true
}
