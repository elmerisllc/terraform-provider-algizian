package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// fakeAPI is a small in-memory stand-in for the Algizian management API: just enough behaviour
// (assigned tenant names, null-clears-branding, domain pending then active, secrets shown once) to run the provider through real Terraform.
type fakeAPI struct {
	mu       sync.Mutex
	srv      *httptest.Server
	n        int
	tenants  map[string]map[string]any
	clients  map[string]map[string]any // by tenant/id
	own      map[string]map[string]any // tenant/provider -> credentials
	email    map[string]map[string]any
	accounts map[string]map[string]any // org/id
	tokens   map[string]map[string]any // org/id
	refresh  map[string]int
}

func newFake() *fakeAPI {
	f := &fakeAPI{tenants: map[string]map[string]any{}, clients: map[string]map[string]any{}, own: map[string]map[string]any{}, email: map[string]map[string]any{},
		accounts: map[string]map[string]any{}, tokens: map[string]map[string]any{}, refresh: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *fakeAPI) problem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"title": http.StatusText(status), "status": status, "detail": detail})
}

func (f *fakeAPI) tenantOut(t map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range t {
		out[k] = v
	}
	if d, ok := t["customDomain"].(map[string]any); ok {
		out["customDomain"] = map[string]any{"hostname": d["hostname"], "status": d["status"], "sslStatus": "active", "error": "",
			"dns": map[string]any{"type": "CNAME", "name": d["hostname"], "target": "cname.algizian.test"}, "issuer": "https://" + d["hostname"].(string)}
	}
	return out
}

func (f *fakeAPI) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tok := r.Header.Get("Authorization")
	if tok != "Bearer ag_test" && tok != "Bearer ag_platform" {
		f.problem(w, 401, "bad token")
		return
	}
	var body map[string]any
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&body)
	}
	if body == nil {
		body = map[string]any{}
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/v1")
	seg := strings.Split(strings.Trim(p, "/"), "/")
	out := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(v)
	}
	m := r.Method
	switch {
	// ---- tenants
	case m == "GET" && len(seg) == 3 && seg[0] == "orgs" && seg[2] == "tenants":
		list := []any{}
		for _, t := range f.tenants {
			if t["org"] != seg[1] || !hasTags(t["tags"], r.URL.Query()["tag"]) {
				continue
			}
			list = append(list, f.tenantOut(t))
		}
		out(200, map[string]any{"tenants": list})
	case m == "POST" && len(seg) == 3 && seg[0] == "orgs" && seg[2] == "tenants":
		if _, has := body["slug"]; has {
			f.problem(w, 400, "The name is assigned: do not send a slug.")
			return
		}
		f.n++
		slug := fmt.Sprintf("t%06d", f.n)
		f.createTenant(slug, seg[1], body)
		out(201, f.tenantOut(f.tenants[slug]))
	case m == "POST" && p == "/tenants":
		slug, _ := body["slug"].(string)
		if len(slug) < 4 {
			f.problem(w, 400, "The slug must be 4 to 40 characters.")
			return
		}
		f.createTenant(slug, "", body)
		out(201, f.tenantOut(f.tenants[slug]))
	case len(seg) >= 2 && seg[0] == "tenants":
		f.tenantRoutes(w, m, seg, body, out)
	// ---- organizations
	case len(seg) >= 3 && seg[0] == "orgs":
		f.orgRoutes(w, m, seg, body, out)
	case m == "GET" && p == "/me":
		if tok == "Bearer ag_platform" {
			out(200, map[string]any{"scope": "platform", "org": nil})
		} else {
			out(200, map[string]any{"scope": "manage", "org": "acme"})
		}
	case m == "GET" && p == "/providers":
		out(200, map[string]any{"providers": []any{map[string]any{"provider": "google", "displayName": "Google"}, map[string]any{"provider": "email", "displayName": "Email"}}})
	default:
		f.problem(w, 404, "no such route "+m+" "+p)
	}
}

func (f *fakeAPI) createTenant(slug, org string, body map[string]any) {
	branding := map[string]any{}
	if b, ok := body["branding"].(map[string]any); ok {
		for k, v := range b {
			branding[k] = v
		}
	}
	name, _ := body["name"].(string)
	if name == "" {
		if d, ok := branding["displayName"].(string); ok {
			name = d
		} else {
			name = slug
		}
	}
	providers := body["providers"]
	if providers == nil {
		providers = []any{"google", "microsoft", "apple", "email"}
	}
	tags := map[string]any{}
	if tg, ok := body["tags"].(map[string]any); ok {
		tags = tg
	}
	t := map[string]any{"slug": slug, "tags": tags, "name": name, "issuer": "https://" + slug + ".algizian.test", "branding": branding, "providers": providers, "locked": body["locked"] == true, "createdAt": time.Now().UTC().Format(time.RFC3339)}
	if org != "" {
		t["org"] = org
	}
	f.tenants[slug] = t
}

func (f *fakeAPI) tenantRoutes(w http.ResponseWriter, m string, seg []string, body map[string]any, out func(int, any)) {
	slug := seg[1]
	t, ok := f.tenants[slug]
	if !ok {
		f.problem(w, 404, "No such tenant.")
		return
	}
	rest := strings.Join(seg[2:], "/")
	switch {
	case rest == "" && m == "GET":
		out(200, f.tenantOut(t))
	case rest == "" && m == "PATCH":
		if v, ok := body["name"]; ok {
			t["name"] = v
		}
		if v, ok := body["providers"]; ok {
			t["providers"] = v
		}
		if v, ok := body["tags"]; ok {
			t["tags"] = v
		}
		if v, ok := body["locked"]; ok {
			t["locked"] = v
		}
		if b, ok := body["branding"].(map[string]any); ok {
			cur := t["branding"].(map[string]any)
			for k, v := range b {
				if v == nil {
					delete(cur, k)
				} else {
					cur[k] = v
				}
			}
		}
		out(200, f.tenantOut(t))
	case rest == "" && m == "DELETE":
		if t["locked"] == true {
			f.problem(w, 409, "The tenant is locked; set locked to false first, then delete it.")
			return
		}
		delete(f.tenants, slug)
		w.WriteHeader(204)
	case rest == "domain" && m == "PUT":
		t["customDomain"] = map[string]any{"hostname": body["hostname"], "status": "pending"}
		f.refresh[slug] = 0
		out(200, f.tenantOut(t))
	case rest == "domain/refresh" && m == "POST":
		d, ok := t["customDomain"].(map[string]any)
		if !ok {
			f.problem(w, 404, "No custom domain.")
			return
		}
		f.refresh[slug]++
		if f.refresh[slug] >= 1 {
			d["status"] = "active"
		}
		out(200, f.tenantOut(t))
	case rest == "domain" && m == "DELETE":
		delete(t, "customDomain")
		w.WriteHeader(204)
	case rest == "clients" && m == "POST":
		f.n++
		id := fmt.Sprintf("c_%d", f.n)
		c := map[string]any{"id": id, "tenant": slug, "tags": orEmpty(body["tags"]), "name": body["name"], "redirectUris": body["redirectUris"], "type": body["type"], "grantTypes": body["grantTypes"], "firstParty": body["firstParty"], "registration": "api"}
		if c["type"] == nil {
			c["type"] = "public"
		}
		f.clients[slug+"/"+id] = c
		res := map[string]any{}
		for k, v := range c {
			res[k] = v
		}
		if c["type"] == "confidential" {
			res["clientSecret"] = "cs_" + id
		}
		out(201, res)
	case strings.HasPrefix(rest, "clients/"):
		key := slug + "/" + strings.TrimPrefix(rest, "clients/")
		c, ok := f.clients[key]
		if !ok {
			f.problem(w, 404, "No such client.")
			return
		}
		switch m {
		case "GET":
			out(200, c)
		case "PATCH":
			for _, k := range []string{"name", "redirectUris", "firstParty", "grantTypes", "tags"} {
				if v, ok := body[k]; ok {
					c[k] = v
				}
			}
			out(200, c)
		case "DELETE":
			delete(f.clients, key)
			w.WriteHeader(204)
		}
	case rest == "providers" && m == "GET":
		list := []any{}
		for _, pr := range []string{"google", "microsoft", "apple"} {
			e := map[string]any{"provider": pr, "enabled": true, "source": "shared", "available": true}
			if c, ok := f.own[slug+"/"+pr]; ok {
				e["source"], e["clientId"] = "own", c["clientId"]
			}
			list = append(list, e)
		}
		out(200, map[string]any{"providers": list})
	case strings.HasPrefix(rest, "providers/") && strings.HasSuffix(rest, "/credentials"):
		pr := strings.Split(rest, "/")[1]
		if m == "PUT" {
			f.own[slug+"/"+pr] = body
			out(200, map[string]any{"provider": pr})
		} else {
			delete(f.own, slug+"/"+pr)
			w.WriteHeader(204)
		}
	case rest == "email" && m == "PUT":
		cur := map[string]any{"configured": true, "enabled": true, "ready": true, "status": "untested", "template": "minimal", "effectiveDailyCap": 200, "effectiveMonthlyCap": 1000}
		for k, v := range body {
			if k != "password" {
				cur[k] = v
			}
		}
		if body["sender"] == "platform" {
			cur["fromAddress"] = "no-reply@algizian.test"
		}
		f.email[slug] = cur
		out(200, cur)
	case rest == "email" && m == "GET":
		if e, ok := f.email[slug]; ok {
			out(200, e)
		} else {
			out(200, map[string]any{"configured": false})
		}
	case rest == "email" && m == "DELETE":
		delete(f.email, slug)
		w.WriteHeader(204)
	default:
		f.problem(w, 404, "no such route")
	}
}

func (f *fakeAPI) orgRoutes(w http.ResponseWriter, m string, seg []string, body map[string]any, out func(int, any)) {
	org := seg[1]
	switch {
	case seg[2] == "service-accounts" && len(seg) == 3 && m == "POST":
		f.n++
		id := fmt.Sprintf("sa_%d", f.n)
		f.accounts[org+"/"+id] = map[string]any{"id": id, "name": body["name"], "role": body["role"], "tags": orEmpty(body["tags"])}
		out(201, f.accounts[org+"/"+id])
	case seg[2] == "service-accounts" && len(seg) == 3 && m == "GET":
		list := []any{}
		for k, v := range f.accounts {
			if strings.HasPrefix(k, org+"/") {
				list = append(list, v)
			}
		}
		out(200, map[string]any{"serviceAccounts": list})
	case seg[2] == "service-accounts" && len(seg) == 4:
		key := org + "/" + seg[3]
		a, ok := f.accounts[key]
		if !ok {
			f.problem(w, 404, "No such service account.")
			return
		}
		if m == "PATCH" {
			for k, v := range body {
				a[k] = v
			}
			out(200, a)
		} else {
			delete(f.accounts, key)
			w.WriteHeader(204)
		}
	case seg[2] == "service-accounts" && len(seg) == 5 && seg[4] == "tokens" && m == "POST":
		f.n++
		id := fmt.Sprintf("tok%d", f.n)
		f.tokens[org+"/"+id] = map[string]any{"id": id, "name": body["name"], "role": "admin", "prefix": "ag_" + id[:4], "serviceAccount": seg[3], "tags": orEmpty(body["tags"]), "createdAt": time.Now().UTC().Format(time.RFC3339)}
		res := map[string]any{"token": "ag_secret_" + id}
		for k, v := range f.tokens[org+"/"+id] {
			res[k] = v
		}
		out(201, res)
	case seg[2] == "tokens" && len(seg) == 3 && m == "GET":
		list := []any{}
		for k, v := range f.tokens {
			if strings.HasPrefix(k, org+"/") {
				list = append(list, v)
			}
		}
		out(200, map[string]any{"tokens": list})
	case seg[2] == "tokens" && len(seg) == 4 && m == "DELETE":
		delete(f.tokens, org+"/"+seg[3])
		w.WriteHeader(204)
	default:
		f.problem(w, 404, "no such route")
	}
}

func orEmpty(v any) any {
	if v == nil {
		return map[string]any{}
	}
	return v
}

// hasTags implements ?tag=key=value (and ?tag=key) like the real API.
func hasTags(have any, filters []string) bool {
	m, _ := have.(map[string]any)
	for _, fl := range filters {
		k, v, hasV := strings.Cut(fl, "=")
		got, ok := m[k]
		if !ok || (hasV && got != v) {
			return false
		}
	}
	return true
}
