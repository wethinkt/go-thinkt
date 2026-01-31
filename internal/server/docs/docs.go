// Package docs contains the generated swagger documentation.
// Run `swag init -g internal/server/api.go -o internal/server/docs` to regenerate.
package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "swagger": "2.0",
    "info": {
        "title": "Thinkt API",
        "description": "API for exploring AI conversation traces from Claude Code, Kimi Code, and other sources.",
        "version": "1.0"
    },
    "host": "localhost:7433",
    "basePath": "/api/v1",
    "paths": {
        "/sources": {
            "get": {
                "description": "Returns all configured trace sources (e.g., Claude Code, Kimi Code)",
                "produces": ["application/json"],
                "tags": ["sources"],
                "summary": "List available trace sources",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/SourcesResponse"
                        }
                    }
                }
            }
        },
        "/projects": {
            "get": {
                "description": "Returns all projects from all sources, optionally filtered by source",
                "produces": ["application/json"],
                "tags": ["projects"],
                "summary": "List all projects",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Filter by source (e.g., 'claude', 'kimi')",
                        "name": "source",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/ProjectsResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/ErrorResponse"
                        }
                    }
                }
            }
        },
        "/projects/{projectID}/sessions": {
            "get": {
                "description": "Returns all sessions belonging to a specific project",
                "produces": ["application/json"],
                "tags": ["sessions"],
                "summary": "List sessions for a project",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Project ID (URL-encoded path)",
                        "name": "projectID",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/SessionsResponse"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/ErrorResponse"
                        }
                    }
                }
            }
        },
        "/sessions/{path}": {
            "get": {
                "description": "Returns session metadata and entries with optional pagination",
                "produces": ["application/json"],
                "tags": ["sessions"],
                "summary": "Get session content",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Session file path (URL-encoded)",
                        "name": "path",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "integer",
                        "description": "Maximum number of entries to return (0 for all)",
                        "name": "limit",
                        "in": "query"
                    },
                    {
                        "type": "integer",
                        "description": "Number of entries to skip",
                        "name": "offset",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/SessionResponse"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/ErrorResponse"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "SourcesResponse": {
            "type": "object",
            "properties": {
                "sources": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/APISourceInfo"
                    }
                }
            }
        },
        "APISourceInfo": {
            "type": "object",
            "properties": {
                "name": {"type": "string"},
                "available": {"type": "boolean"},
                "base_path": {"type": "string"}
            }
        },
        "ProjectsResponse": {
            "type": "object",
            "properties": {
                "projects": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/Project"
                    }
                }
            }
        },
        "Project": {
            "type": "object",
            "properties": {
                "id": {"type": "string"},
                "name": {"type": "string"},
                "path": {"type": "string"},
                "session_count": {"type": "integer"},
                "source": {"type": "string"}
            }
        },
        "SessionsResponse": {
            "type": "object",
            "properties": {
                "sessions": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/SessionMeta"
                    }
                }
            }
        },
        "SessionMeta": {
            "type": "object",
            "properties": {
                "id": {"type": "string"},
                "full_path": {"type": "string"},
                "entry_count": {"type": "integer"},
                "file_size": {"type": "integer"},
                "source": {"type": "string"},
                "created_at": {"type": "string", "format": "date-time"},
                "modified_at": {"type": "string", "format": "date-time"}
            }
        },
        "SessionResponse": {
            "type": "object",
            "properties": {
                "meta": {"$ref": "#/definitions/SessionMeta"},
                "entries": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/Entry"
                    }
                },
                "has_more": {"type": "boolean"},
                "total": {"type": "integer"}
            }
        },
        "Entry": {
            "type": "object",
            "properties": {
                "uuid": {"type": "string"},
                "role": {"type": "string"},
                "timestamp": {"type": "string", "format": "date-time"},
                "text": {"type": "string"},
                "model": {"type": "string"}
            }
        },
        "ErrorResponse": {
            "type": "object",
            "properties": {
                "error": {"type": "string"},
                "message": {"type": "string"}
            }
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "1.0",
	Host:             "localhost:7433",
	BasePath:         "/api/v1",
	Schemes:          []string{},
	Title:            "Thinkt API",
	Description:      "API for exploring AI conversation traces from Claude Code, Kimi Code, and other sources.",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
