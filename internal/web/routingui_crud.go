package web

import (
	"fmt"
	"net/http"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// validatable — элемент routing-конфига, умеющий проверять себя (Inbound,
// Outbound, RuleSetRef). Общий constraint для generic CRUD-хелперов ниже.
type validatable interface {
	Validate() error
}

// crudSpec параметризует generic tag-keyed CRUD над одним срезом
// RoutingConfig (Inbounds/Outbounds/RuleSets). get/set дают доступ к
// конкретному срезу конфига, tagOf извлекает ключ элемента (поле Tag),
// entity — имя сущности для текстов ошибок ("inbound"/"outbound"/"rule_set").
type crudSpec[T validatable] struct {
	entity string
	get    func(*types.RoutingConfig) []T
	set    func(*types.RoutingConfig, []T)
	tagOf  func(T) string
}

// crudList — общая реализация GET-хендлера: отдаёт срез конфига как есть.
func crudList[T validatable](s *Server, w http.ResponseWriter, r *http.Request, spec crudSpec[T]) {
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, spec.get(cfg))
}

// crudAdd — общая реализация POST-хендлера: декодирует тело, валидирует,
// проверяет уникальность Tag (409 при конфликте) и добавляет элемент.
func crudAdd[T validatable](s *Server, w http.ResponseWriter, r *http.Request, spec crudSpec[T]) {
	var item T
	if !decodeJSON(w, r, &item) {
		return
	}
	if err := item.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := spec.get(cfg)
	tag := spec.tagOf(item)
	for _, existing := range items {
		if spec.tagOf(existing) == tag {
			http.Error(w, fmt.Sprintf("%s с таким tag уже существует", spec.entity), http.StatusConflict)
			return
		}
	}
	spec.set(cfg, append(items, item))
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, item)
}

// crudUpdate — общая реализация PUT-хендлера: декодирует тело, валидирует,
// ищет элемент по Tag из пути (404, если не найден) и заменяет его целиком.
func crudUpdate[T validatable](s *Server, w http.ResponseWriter, r *http.Request, spec crudSpec[T]) {
	tag := r.PathValue("tag")
	var item T
	if !decodeJSON(w, r, &item) {
		return
	}
	if err := item.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := spec.get(cfg)
	idx := -1
	for i, existing := range items {
		if spec.tagOf(existing) == tag {
			idx = i
			break
		}
	}
	if idx < 0 {
		http.Error(w, fmt.Sprintf("%s не найден", spec.entity), http.StatusNotFound)
		return
	}
	items[idx] = item
	spec.set(cfg, items)
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, item)
}

// crudDelete — общая реализация DELETE-хендлера: убирает элемент с данным
// Tag из пути (404, если не найден). Переиспользует backing-массив исходного
// среза (as `items[:0]`) — как и в исходной ручной реализации.
func crudDelete[T validatable](s *Server, w http.ResponseWriter, r *http.Request, spec crudSpec[T]) {
	tag := r.PathValue("tag")
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := spec.get(cfg)
	out := items[:0]
	found := false
	for _, existing := range items {
		if spec.tagOf(existing) == tag {
			found = true
			continue
		}
		out = append(out, existing)
	}
	if !found {
		http.Error(w, fmt.Sprintf("%s не найден", spec.entity), http.StatusNotFound)
		return
	}
	spec.set(cfg, out)
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
