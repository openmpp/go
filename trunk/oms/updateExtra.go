// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"errors"

	"go.openmpp.org/ompp/db"
	"go.openmpp.org/ompp/omppLog"
)

// ReplaceProfile replace existing or insert new profile and all profile options.
func (mc *ModelCatalog) ReplaceProfile(dn string, pm *db.ProfileMeta) (bool, error) {

	// if model digest-or-name or profile name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return false, nil
	}
	if pm.Name == "" {
		omppLog.Log("Warning: invalid (empty) profile name")
		return false, nil
	}

	// if model metadata not loaded then read it from database
	idx, ok := mc.loadModelMeta(dn)
	if !ok {
		omppLog.Log("Error: model digest or name not found: ", dn)
		return false, errors.New("Error: model digest or name not found: " + dn)
	}

	// lock catalog and update profile
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	err := db.UpdateProfile(mc.modelLst[idx].dbConn, pm)
	if err != nil {
		omppLog.Log("Error at update profile: ", dn, ": ", pm.Name, ": ", err.Error())
		return false, err
	}

	return true, nil
}

// DeleteProfile delete profile and all profile options.
// If multiple models with same name exist then result is undefined.
// If no such profile exist in database then no error, empty operation.
func (mc *ModelCatalog) DeleteProfile(dn, profile string) (bool, error) {

	// if model digest-or-name or profile name is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return false, nil
	}
	if profile == "" {
		omppLog.Log("Warning: invalid (empty) profile name")
		return false, nil
	}

	// if model metadata not loaded then read it from database
	idx, ok := mc.loadModelMeta(dn)
	if !ok {
		omppLog.Log("Error: model digest or name not found: ", dn)
		return false, errors.New("Error: model digest or name not found: " + dn)
	}

	// lock catalog and update profile
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	err := db.DeleteProfile(mc.modelLst[idx].dbConn, profile)
	if err != nil {
		omppLog.Log("Error at update profile: ", dn, ": ", profile, ": ", err.Error())
		return false, err
	}

	return true, nil
}

// ReplaceProfileOption insert new or replace existsing profile and profile option key-value.
// If multiple models with same name exist then result is undefined.
// If no such profile or option exist in database then new profile and option inserted.
func (mc *ModelCatalog) ReplaceProfileOption(dn, profile, key, val string) (bool, error) {

	// if model digest-or-name, profile name or option key is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return false, nil
	}
	if profile == "" {
		omppLog.Log("Warning: invalid (empty) profile name")
		return false, nil
	}
	if key == "" {
		omppLog.Log("Warning: invalid (empty) profile option key")
		return false, nil
	}

	// if model metadata not loaded then read it from database
	idx, ok := mc.loadModelMeta(dn)
	if !ok {
		omppLog.Log("Error: model digest or name not found: ", dn)
		return false, errors.New("Error: model digest or name not found: " + dn)
	}

	// lock catalog and update profile option
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	err := db.UpdateProfileOption(mc.modelLst[idx].dbConn, profile, key, val)
	if err != nil {
		omppLog.Log("Error at update profile option: ", dn, ": ", profile, ": ", ": ", key, err.Error())
		return false, err
	}

	return true, nil
}

// DeleteProfileOption delete profile option key-value pair.
// If multiple models with same name exist then result is undefined.
// If no such profile or profile option key exist in database then no error, empty operation.
func (mc *ModelCatalog) DeleteProfileOption(dn, profile, key string) (bool, error) {

	// if model digest-or-name, profile name or option key is empty then return empty results
	if dn == "" {
		omppLog.Log("Warning: invalid (empty) model digest and name")
		return false, nil
	}
	if profile == "" {
		omppLog.Log("Warning: invalid (empty) profile name")
		return false, nil
	}
	if key == "" {
		omppLog.Log("Warning: invalid (empty) profile option key")
		return false, nil
	}

	// if model metadata not loaded then read it from database
	idx, ok := mc.loadModelMeta(dn)
	if !ok {
		omppLog.Log("Error: model digest or name not found: ", dn)
		return false, errors.New("Error: model digest or name not found: " + dn)
	}

	// lock catalog and delete profile option
	mc.theLock.Lock()
	defer mc.theLock.Unlock()

	err := db.DeleteProfileOption(mc.modelLst[idx].dbConn, profile, key)
	if err != nil {
		omppLog.Log("Error at delete profile option: ", dn, ": ", profile, ": ", ": ", key, err.Error())
		return false, err
	}

	return true, nil
}
