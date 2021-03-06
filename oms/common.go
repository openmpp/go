// Copyright (c) 2016 OpenM++
// This code is licensed under the MIT license (see LICENSE.txt for details)

package main

import (
	"container/list"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/husobee/vestigo"
	"golang.org/x/text/language"

	"github.com/openmpp/go/ompp/omppLog"
)

// logRequest is a middelware to log http request
func logRequest(next http.HandlerFunc) http.HandlerFunc {
	if isLogRequest {
		return func(w http.ResponseWriter, r *http.Request) {
			omppLog.Log(r.Method, ": ", r.Host, r.URL)
			next(w, r)
		}
	} // else
	return next
}

// match request language with UI supported languages and return canonic language name
func matchRequestToUiLang(r *http.Request) string {
	rqLangTags, _, _ := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
	tag, _, _ := uiLangMatcher.Match(rqLangTags...)
	return tag.String()
}

// get value of url parameter ?name or router parameter /:name
func getRequestParam(r *http.Request, name string) string {

	v := r.URL.Query().Get(name)
	if v == "" {
		v = vestigo.Param(r, name)
	}
	return v
}

// get boolean value of url parameter ?name or router parameter /:name
func getBoolRequestParam(r *http.Request, name string) (bool, bool) {

	v := r.URL.Query().Get(name)
	if v == "" {
		v = vestigo.Param(r, name)
	}
	if v == "" {
		return false, true // no such parameter: return = false by default
	}
	if isVal, err := strconv.ParseBool(v); err == nil {
		return isVal, true // return result: value is boolean
	}
	return false, false // value is not boolean
}

// get integer value of url parameter ?name or router parameter /:name
func getIntRequestParam(r *http.Request, name string, defaultVal int) (int, bool) {

	v := r.URL.Query().Get(name)
	if v == "" {
		v = vestigo.Param(r, name)
	}
	if v == "" {
		return defaultVal, true // no such parameter: return defult value
	}
	if nVal, err := strconv.Atoi(v); err == nil {
		return nVal, true // return result: value is integer
	}
	return defaultVal, false // value is not integer
}

// get int64 value of url parameter ?name or router parameter /:name
func getInt64RequestParam(r *http.Request, name string, defaultVal int64) (int64, bool) {

	v := r.URL.Query().Get(name)
	if v == "" {
		v = vestigo.Param(r, name)
	}
	if v == "" {
		return defaultVal, true // no such parameter: return defult value
	}
	if nVal, err := strconv.ParseInt(v, 0, 64); err == nil {
		return nVal, true // return result: value is integer
	}
	return defaultVal, false // value is not integer
}

// get languages accepted by browser and by optional language request parameter, for example: ..../lang:EN
// if language parameter specified then return it as a first element of result (it a preferred language)
func getRequestLang(r *http.Request, name string) []language.Tag {

	// browser languages
	rqLangTags, _, _ := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))

	// if optional url parameter ?lang or router parameter /:lang specified
	ln := r.URL.Query().Get(name)
	if ln == "" {
		ln = vestigo.Param(r, name)
	}

	// add lang parameter as top language
	if ln != "" {
		if t := language.Make(ln); t != language.Und {
			rqLangTags = append([]language.Tag{t}, rqLangTags...)
		}
	}
	return rqLangTags
}

// set Content-Type header by extension and invoke next handler.
// This function exist to suppress Windows registry content type overrides
func setContentType(next http.Handler) http.Handler {

	var ctDef = map[string]string{
		".css": "text/css; charset=utf-8",
		".js":  "text/javascript",
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if ext := filepath.Ext(r.URL.Path); ext != "" {
			if ct := ctDef[strings.ToLower(ext)]; ct != "" {
				w.Header().Set("Content-Type", ct)
			}
		}
		next.ServeHTTP(w, r) // invoke next handler
	})
}

// jsonSetHeaders set jscon response headers: Content-Type: application/json and Access-Control-Allow-Origin
func jsonSetHeaders(w http.ResponseWriter, r *http.Request) {

	// if Content-Type not set then use json
	if _, isSet := w.Header()["Content-Type"]; !isSet {
		w.Header().Set("Content-Type", "application/json")
	}

	// if request from localhost then allow response to any protocol or port
	/*
		if strings.HasPrefix(r.Host, "localhost") {
			if _, isSet := w.Header()["Access-Control-Allow-Origin"]; !isSet {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
		}
	*/
}

// jsonResponse set json response headers and writes src as json into w response writer.
// On error it writes 500 internal server error response.
func jsonResponse(w http.ResponseWriter, r *http.Request, src interface{}) {

	jsonSetHeaders(w, r)

	err := json.NewEncoder(w).Encode(src)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// jsonResponseBytes set json response headers and writes src bytes into w response writer.
// If src bytes empty then "{}" is written.
func jsonResponseBytes(w http.ResponseWriter, r *http.Request, src []byte) {

	jsonSetHeaders(w, r)

	if len(src) <= 0 {
		w.Write([]byte("{}"))
	}
	w.Write(src)
}

// jsonAppendListToResponse writes srcLst as json array into w response writer.
// It is an append to response and response headers must already be set.
// On error it writes 500 internal server error response.
func jsonAppendListToResponse(
	w http.ResponseWriter, r *http.Request, srcLst *list.List, cvt func(interface{}) (interface{}, error)) {

	w.Write([]byte{'['}) // start json output array

	enc := json.NewEncoder(w)
	isNext := false

	for src := srcLst.Front(); src != nil; src = src.Next() {

		if isNext {
			w.Write([]byte{','}) // until the last separate array items with , comma
		}

		val := src.Value // id's cell
		var err error

		// convert cell from id's to code if converter specified
		if cvt != nil {
			if val, err = cvt(val); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}

		// write actual value
		if err := enc.Encode(val); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		isNext = true
	}

	w.Write([]byte{']'}) // end of array
}

// jsonRequestDecode validate Content-Type: application/json and decode json body.
// Destination for json decode: dst must be a pointer.
// On error it writes error response 400 or 415 and return false.
func jsonRequestDecode(w http.ResponseWriter, r *http.Request, dst interface{}) bool {

	// json body expected
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "Expected Content-Type: application/json", http.StatusUnsupportedMediaType)
		return false
	}

	// decode json
	err := json.NewDecoder(r.Body).Decode(dst)
	if err != nil {
		if err == io.EOF {
			http.Error(w, "Invalid (empty) json at "+r.URL.String(), http.StatusBadRequest)
			return false
		}
		http.Error(w, "Json decode error at "+r.URL.String(), http.StatusBadRequest)
		return false
	}
	return true // completed OK
}

// jsonMultipartDecode decode json from multipart form reader.
// It does move to next part, check part form name and decode json from part body.
// Destination for json decode: dst must be a pointer.
// On error it writes error response 400 or 415 and return false.
func jsonMultipartDecode(w http.ResponseWriter, mr *multipart.Reader, name string, dst interface{}) bool {

	// open next part and check part name
	part, err := mr.NextPart()
	if err == io.EOF {
		http.Error(w, "Invalid (empty) next part of multipart form "+name, http.StatusBadRequest)
		return false
	}
	if err != nil {
		http.Error(w, "Failed to get next part of multipart form "+name+" : "+err.Error(), http.StatusBadRequest)
		return false
	}
	defer part.Close()

	if part.FormName() != name {
		http.Error(w, "Invalid part of multipart form, expected name: "+name, http.StatusBadRequest)
		return false
	}

	// decode json
	err = json.NewDecoder(part).Decode(dst)
	if err != nil {
		if err == io.EOF {
			http.Error(w, "Invalid (empty) json part of multipart form "+name, http.StatusBadRequest)
			return false
		}
		http.Error(w, "Json decode error at part of multipart form "+name, http.StatusBadRequest)
		return false
	}
	return true // completed OK
}

// jsonRequestToFile validate Content-Type: application/json and copy request body into output file as is.
// If output file already exists then it is truncated.
// On error it writes error response 400 or 500 and return false.
func jsonRequestToFile(w http.ResponseWriter, r *http.Request, outPath string) bool {

	// json body expected
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "Expected Content-Type: application/json", http.StatusUnsupportedMediaType)
		return false
	}

	// create or truncate output file
	fName := filepath.Base(outPath)

	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		omppLog.Log("Error: unable to create or truncate file ", outPath, err)
		http.Error(w, "Error: unable to create or truncate file "+fName, http.StatusInternalServerError)
		return false
	}
	defer f.Close()

	// copy request body into the file
	_, err = io.Copy(f, r.Body)
	if err != nil {
		omppLog.Log("Error: unable to write into ", outPath, err)
		http.Error(w, "Error: unable to write into "+fName, http.StatusInternalServerError)
		return false
	}
	return true // completed OK
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
