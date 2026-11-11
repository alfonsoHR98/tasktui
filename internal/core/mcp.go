package core

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Parte común del servidor MCP (JSON-RPC sobre HTTP): herramientas, su traducción
// a llamadas de la API y las respuestas del protocolo. No hace E/S.

// Version es la versión de tasktui (la sobrescribe el build con -ldflags).
var Version = "0.3.0"

const sintaxis = "Captura rápida: fecha («hoy», «mañana», «viernes», «20/11»), hora («18:00»), " +
	"prioridad p1 (máxima) a p4, +Proyecto, #etiqueta, fecha límite con «limit 20/11» y repetición " +
	"(«cada lunes», «cada mes»). Ejemplo: «Pagar luz mañana 18:00 p1 +Finanzas #casa»."

type obj = map[string]any

func str(d string) obj { return obj{"type": "string", "description": d} }

// MCPTools devuelve la lista de herramientas (para tools/list).
func MCPTools() []obj {
	schema := func(props obj, required ...string) obj {
		s := obj{"type": "object", "properties": props}
		if len(required) > 0 {
			s["required"] = required
		}
		return s
	}
	id := str("Id de la tarea")
	return []obj{
		{"name": "list_tasks",
			"description": "Lista tareas agrupadas. Vistas: today, inbox, upcoming, all, +Proyecto (p. ej. +Niuko Consultores) o #etiqueta. Por defecto solo pendientes.",
			"inputSchema": schema(obj{
				"view":         str("today (por defecto), inbox, upcoming, all, +Proyecto o #etiqueta"),
				"query":        str("Texto a buscar"),
				"include_done": obj{"type": "boolean", "description": "Incluir las completadas"}})},
		{"name": "get_task", "description": "Devuelve una tarea completa (descripción y notas incluidas).",
			"inputSchema": schema(obj{"id": id}, "id")},
		{"name": "add_task",
			"description": "Crea una tarea. " + sintaxis + " Sin +Proyecto va a Inbox, o al `project` indicado.",
			"inputSchema": schema(obj{
				"text":        str("Título con captura rápida"),
				"project":     str("Proyecto (opcional)"),
				"description": str("Descripción larga (opcional)")}, "text")},
		{"name": "update_task",
			"description": "Edita una tarea. `text` cambia solo lo que escribas (p. ej. «viernes p2» solo cambia fecha y prioridad). " + sintaxis,
			"inputSchema": schema(obj{
				"id":          id,
				"text":        str("Cambios en captura rápida"),
				"description": str("Reemplaza la descripción"),
				"no_date":     obj{"type": "boolean", "description": "Quita fecha, hora y repetición"},
				"no_deadline": obj{"type": "boolean", "description": "Quita la fecha límite"}}, "id")},
		{"name": "complete_task",
			"description": "Completa una tarea, o la reabre si ya estaba completada. Si es recurrente crea la siguiente.",
			"inputSchema": schema(obj{"id": id}, "id")},
		{"name": "add_note", "description": "Añade una nota de seguimiento con la fecha de hoy.",
			"inputSchema": schema(obj{"id": id, "text": str("Nota")}, "id", "text")},
		{"name": "delete_task", "description": "Elimina una tarea definitivamente.",
			"inputSchema": schema(obj{"id": id}, "id")},
		{"name": "list_projects", "description": "Lista los proyectos con su número de tareas pendientes.",
			"inputSchema": schema(obj{})},
		{"name": "create_project", "description": "Crea un proyecto.",
			"inputSchema": schema(obj{"name": str("Nombre")}, "name")},
		{"name": "delete_project", "description": "Elimina un proyecto y todas sus tareas definitivamente. Inbox no se puede eliminar.",
			"inputSchema": schema(obj{"name": str("Nombre")}, "name")},
		{"name": "list_labels", "description": "Lista las etiquetas con tareas pendientes.",
			"inputSchema": schema(obj{})},
	}
}

func argStr(a obj, k string) (string, bool) {
	s, ok := a[k].(string)
	return s, ok
}

func need(a obj, k string) (string, error) {
	s, ok := argStr(a, k)
	if !ok || strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("Falta «%s»", k)
	}
	return s, nil
}

// MCPCall es la llamada a la API que corresponde a una herramienta.
type MCPCall struct {
	Method string
	Path   string
	Body   any // nil si no hay cuerpo
}

// PlanTool traduce una herramienta MCP a una llamada de la API.
func PlanTool(name string, a obj) (MCPCall, error) {
	enc := url.QueryEscape
	pathEnc := func(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }
	withID := func(f func(id string) MCPCall) (MCPCall, error) {
		id, err := need(a, "id")
		if err != nil {
			return MCPCall{}, err
		}
		return f(id), nil
	}
	switch name {
	case "list_tasks":
		view, ok := argStr(a, "view")
		if !ok {
			view = "today"
		}
		p := "/api/tasks?view=" + enc(view)
		if q, ok := argStr(a, "query"); ok {
			p += "&q=" + enc(q)
		}
		if b, _ := a["include_done"].(bool); b {
			p += "&all=1"
		}
		return MCPCall{"GET", p, nil}, nil
	case "get_task":
		return withID(func(id string) MCPCall { return MCPCall{"GET", "/api/tasks/" + pathEnc(id), nil} })
	case "add_task":
		text, err := need(a, "text")
		if err != nil {
			return MCPCall{}, err
		}
		b := obj{"text": text}
		for _, k := range []string{"project", "description"} {
			if v, ok := argStr(a, k); ok {
				b[k] = v
			}
		}
		return MCPCall{"POST", "/api/tasks", b}, nil
	case "update_task":
		b := obj{}
		for _, k := range []string{"text", "description"} {
			if v, ok := argStr(a, k); ok {
				b[k] = v
			}
		}
		for _, k := range []string{"no_date", "no_deadline"} {
			if v, ok := a[k].(bool); ok {
				b[k] = v
			}
		}
		return withID(func(id string) MCPCall { return MCPCall{"PATCH", "/api/tasks/" + pathEnc(id), b} })
	case "complete_task":
		return withID(func(id string) MCPCall { return MCPCall{"POST", "/api/tasks/" + pathEnc(id) + "/done", nil} })
	case "add_note":
		text, err := need(a, "text")
		if err != nil {
			return MCPCall{}, err
		}
		return withID(func(id string) MCPCall {
			return MCPCall{"POST", "/api/tasks/" + pathEnc(id) + "/notes", obj{"text": text}}
		})
	case "delete_task":
		return withID(func(id string) MCPCall { return MCPCall{"DELETE", "/api/tasks/" + pathEnc(id), nil} })
	case "list_projects":
		return MCPCall{"GET", "/api/projects", nil}, nil
	case "create_project":
		n, err := need(a, "name")
		if err != nil {
			return MCPCall{}, err
		}
		return MCPCall{"POST", "/api/projects", obj{"name": n}}, nil
	case "delete_project":
		n, err := need(a, "name")
		if err != nil {
			return MCPCall{}, err
		}
		return MCPCall{"DELETE", "/api/projects?name=" + pathEnc(n), nil}, nil
	case "list_labels":
		return MCPCall{"GET", "/api/labels", nil}, nil
	}
	return MCPCall{}, fmt.Errorf("Herramienta desconocida «%s»", name)
}

// RPC es el resultado de interpretar un mensaje JSON-RPC.
type RPC struct {
	// Notification: se responde 202 sin cuerpo.
	Notification bool
	// Reply: respuesta lista (si no es nil).
	Reply any
	// Si Call != nil hay que ejecutar la herramienta y responder con ToolReply.
	Call *RPCCall
}

// RPCCall es una herramienta pedida por la IA.
type RPCCall struct {
	ID   json.RawMessage
	Name string
	Args obj
}

// Dispatch interpreta un mensaje JSON-RPC.
func Dispatch(msg map[string]json.RawMessage) RPC {
	id, hasID := msg["id"]
	if !hasID {
		return RPC{Notification: true}
	}
	var method string
	_ = json.Unmarshal(msg["method"], &method)
	var params obj
	_ = json.Unmarshal(msg["params"], &params)
	reply := func(result any) RPC {
		return RPC{Reply: obj{"jsonrpc": "2.0", "id": id, "result": result}}
	}
	switch method {
	case "initialize":
		v, ok := params["protocolVersion"].(string)
		if !ok {
			v = "2025-03-26"
		}
		return reply(obj{
			"protocolVersion": v,
			"capabilities":    obj{"tools": obj{"listChanged": false}},
			"serverInfo":      obj{"name": "tasktui", "version": Version},
			"instructions":    "Tareas personales. Usa list_tasks para consultar y add_task con captura rápida para crear.",
		})
	case "ping":
		return reply(obj{})
	case "tools/list":
		return reply(obj{"tools": MCPTools()})
	case "tools/call":
		name, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]any)
		if args == nil {
			args = obj{}
		}
		return RPC{Call: &RPCCall{ID: id, Name: name, Args: args}}
	}
	return RPC{Reply: obj{"jsonrpc": "2.0", "id": id,
		"error": obj{"code": -32601, "message": "Método desconocido: " + method}}}
}

// ToolReply es la respuesta JSON-RPC al resultado de una herramienta (err = error para la IA).
func ToolReply(id json.RawMessage, out string, err error) any {
	var result obj
	if err != nil {
		result = obj{"isError": true, "content": []obj{{"type": "text", "text": err.Error()}}}
	} else {
		result = obj{"content": []obj{{"type": "text", "text": out}}}
	}
	return obj{"jsonrpc": "2.0", "id": id, "result": result}
}

// MCPParseError es la respuesta para un cuerpo que no es JSON.
func MCPParseError() any {
	return obj{"jsonrpc": "2.0", "id": nil, "error": obj{"code": -32700, "message": "JSON inválido"}}
}
