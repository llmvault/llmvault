package main

import "encoding/json"

const grafanaSpecRevision = "8d4b3ab5a86a3c41df79458e7dd42f5889a6cefe"

func grafanaService() ServiceConfig {
	return ServiceConfig{
		Name:           "grafana",
		SpecSource:     "https://raw.githubusercontent.com/grafana/grafana/" + grafanaSpecRevision + "/public/api-merged.json",
		NangoProviders: []string{"grafana"},
		OperationSelectors: []OperationSelector{
			{Method: "GET", Path: "/datasources"},
			{Method: "GET", Path: "/search"},
			{Method: "GET", Path: "/dashboards/uid/{uid}", AllowDeprecated: true},
			{Method: "POST", Path: "/ds/query"},
		},
		ActionOverrides: map[string]ActionOverride{
			"getDataSources": {
				Key:         "list_data_sources",
				DisplayName: "List Data Sources",
				Description: "List every Grafana data source visible to the connected service account. Use this before query_data_source when you need a data source UID, name, type, or access mode.",
				Access:      "read",
			},
			"search": {
				Key:         "search_dashboards",
				DisplayName: "Search Dashboards",
				Description: "Search Grafana folders and dashboards by title, type, star status, or permission. Results only include resources visible to the connected service account. Pass a returned dashboard UID to get_dashboard.",
				Access:      "read",
				Parameters:  grafanaSearchParameters(),
			},
			"getDashboardByUID": {
				Key:         "get_dashboard",
				DisplayName: "Get Dashboard",
				Description: "Load one Grafana dashboard by UID, including panels, variables, data source references, and saved query models. Use the saved panel queries as a starting point for query_data_source.",
				Access:      "read",
				Parameters:  grafanaDashboardParameters(),
			},
			"queryMetricsWithExpressions": {
				Key:         "query_data_source",
				DisplayName: "Query Data Source",
				Description: "Run one or more backend data source queries through Grafana. Each query needs a unique refId, datasource.uid, and the fields required by that data source, such as expr for Prometheus or Loki and rawSql for SQL. Grafana uses POST for this read-query endpoint; protect SQL data sources with read-only database credentials.",
				Access:      "read",
				Parameters:  grafanaQueryParameters(),
			},
		},
	}
}

func grafanaDashboardParameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["uid"],
  "properties": {
    "uid": {
      "type": "string",
      "minLength": 1,
      "description": "Dashboard UID returned by search_dashboards. This is the stable UID, not the numeric dashboard ID."
    }
  }
}`)
}

func grafanaSearchParameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "query": {
      "type": "string",
      "description": "Text to match against dashboard and folder titles."
    },
    "type": {
      "type": "string",
      "enum": ["dash-db", "dash-folder"],
      "description": "Restrict results to dashboards or folders."
    },
    "starred": {
      "type": "boolean",
      "description": "Return only starred dashboards when true."
    },
    "permission": {
      "type": "string",
      "enum": ["View", "Edit"],
      "description": "Return resources the service account can view or edit."
    },
    "sort": {
      "type": "string",
      "enum": ["alpha-asc", "alpha-desc"],
      "description": "Sort results by title."
    },
    "limit": {
      "type": "integer",
      "minimum": 1,
      "maximum": 1000,
      "description": "Maximum results to return on one page."
    },
    "page": {
      "type": "integer",
      "minimum": 1,
      "description": "One-based result page."
    }
  }
}`)
}

func grafanaQueryParameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["from", "to", "queries"],
  "properties": {
    "from": {
      "type": "string",
      "description": "Start of the time range as milliseconds since the Unix epoch or a Grafana relative time such as now-1h."
    },
    "to": {
      "type": "string",
      "description": "End of the time range as milliseconds since the Unix epoch or a Grafana relative time such as now."
    },
    "queries": {
      "type": "array",
      "minItems": 1,
      "maxItems": 10,
      "description": "Grafana data source query models. Every item needs a unique refId and a data source UID. Keep provider-specific fields such as expr, rawSql, queryType, editorMode, or legendFormat on the query object.",
      "items": {
        "type": "object",
        "additionalProperties": true,
        "required": ["refId", "datasource"],
        "properties": {
          "refId": {
            "type": "string",
            "minLength": 1,
            "description": "Unique result identifier within this request, commonly A, B, or C."
          },
          "datasource": {
            "type": "object",
            "additionalProperties": true,
            "required": ["uid"],
            "properties": {
              "uid": {
                "type": "string",
                "minLength": 1,
                "description": "Grafana data source UID returned by list_data_sources."
              },
              "type": {
                "type": "string",
                "description": "Optional data source plugin type, such as prometheus, loki, tempo, postgres, or mysql."
              }
            }
          },
          "format": {
            "type": "string",
            "enum": ["time_series", "table"],
            "description": "Requested result format when the data source supports it."
          },
          "maxDataPoints": {
            "type": "integer",
            "minimum": 1,
            "maximum": 10000,
            "description": "Maximum points Grafana should return for this query."
          },
          "intervalMs": {
            "type": "integer",
            "minimum": 1,
            "description": "Query interval in milliseconds."
          },
          "expr": {
            "type": "string",
            "description": "Prometheus, Loki, or other expression understood by the selected data source."
          },
          "rawSql": {
            "type": "string",
            "description": "A SELECT, WITH, SHOW, EXPLAIN, or other read-only statement for a SQL-backed Grafana data source."
          }
        }
      }
    },
    "debug": {
      "type": "boolean",
      "description": "Ask Grafana to include query debugging details when supported."
    }
  }
}`)
}
