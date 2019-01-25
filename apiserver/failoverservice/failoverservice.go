package failoverservice

/*
Copyright 2017-2019 Crunchy Data Solutions, Inc.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import (
	"encoding/json"
	log "github.com/Sirupsen/logrus"
	"github.com/crunchydata/postgres-operator/apiserver"
	msgs "github.com/crunchydata/postgres-operator/apiservermsgs"
	"github.com/gorilla/mux"
	"net/http"
)

// CreateFailoverHandler ...
// pgo failover mycluster
func CreateFailoverHandler(w http.ResponseWriter, r *http.Request) {
	var ns string

	log.Debug("failoverservice.CreateFailoverHandler called")

	var request msgs.CreateFailoverRequest
	_ = json.NewDecoder(r.Body).Decode(&request)

	username, err := apiserver.Authn(apiserver.CREATE_FAILOVER_PERM, w, r)
	if err != nil {
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	resp := msgs.CreateFailoverResponse{}
	resp.Status = msgs.Status{Code: msgs.Ok, Msg: ""}

	if request.ClientVersion != msgs.PGO_VERSION {
		resp.Status = msgs.Status{Code: msgs.Error, Msg: apiserver.VERSION_MISMATCH_ERROR}
		json.NewEncoder(w).Encode(resp)
		return
	}

	ns, err = apiserver.GetNamespace(username, "")
	if err != nil {
		resp.Status = msgs.Status{Code: msgs.Error, Msg: err.Error()}
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp = CreateFailover(&request, ns)

	json.NewEncoder(w).Encode(resp)
}

// QueryFailoverHandler ...
// pgo failover mycluster --query
func QueryFailoverHandler(w http.ResponseWriter, r *http.Request) {
	var ns string

	log.Debug("failoverservice.QueryFailoverHandler called")
	vars := mux.Vars(r)
	name := vars["name"]

	clientVersion := r.URL.Query().Get("version")
	if clientVersion != "" {
		log.Debugf("version parameter is [%s]", clientVersion)
	}

	username, err := apiserver.Authn(apiserver.CREATE_FAILOVER_PERM, w, r)
	if err != nil {
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
	w.Header().Set("Content-Type", "application/json")

	resp := msgs.QueryFailoverResponse{}
	resp.Status = msgs.Status{Code: msgs.Ok, Msg: ""}

	if clientVersion != msgs.PGO_VERSION {
		resp.Status = msgs.Status{Code: msgs.Error, Msg: apiserver.VERSION_MISMATCH_ERROR}
		json.NewEncoder(w).Encode(resp)
		return
	}

	ns, err = apiserver.GetNamespace(username, "")
	if err != nil {
		resp.Status = msgs.Status{Code: msgs.Error, Msg: err.Error()}
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp = QueryFailover(name, ns)
	json.NewEncoder(w).Encode(resp)
}
