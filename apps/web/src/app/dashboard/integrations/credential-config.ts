export type CredField = {
  key: string;
  label: string;
  type: "input" | "textarea";
};

export type CredConfig = {
  fields: CredField[];
  optional?: string[];
  message?: string;
};

export function credentialFieldsForAuthMode(authMode: string): CredConfig {
  switch (authMode) {
    case "OAUTH2":
    case "OAUTH1":
    case "TBA":
      return {
        fields: [
          { key: "client_id", label: "Client ID", type: "input" },
          { key: "client_secret", label: "Client Secret", type: "input" },
          { key: "scopes", label: "Scopes", type: "input" },
        ],
        optional: ["scopes"],
      };
    case "APP":
      return {
        fields: [
          { key: "app_id", label: "App ID", type: "input" },
          { key: "app_link", label: "App Link", type: "input" },
          { key: "private_key", label: "Private Key", type: "textarea" },
        ],
      };
    case "CUSTOM":
      return {
        fields: [
          { key: "client_id", label: "Client ID", type: "input" },
          { key: "client_secret", label: "Client Secret", type: "input" },
          { key: "app_id", label: "App ID", type: "input" },
          { key: "app_link", label: "App Link", type: "input" },
          { key: "private_key", label: "Private Key", type: "textarea" },
        ],
      };
    case "INSTALL_PLUGIN":
      return {
        fields: [{ key: "app_link", label: "App Link", type: "input" }],
      };
    case "MCP_OAUTH2":
      return {
        fields: [
          { key: "client_id", label: "Client ID", type: "input" },
          { key: "client_secret", label: "Client Secret", type: "input" },
        ],
        message: "Required for static client registration.",
      };
    default:
      return {
        fields: [],
        message: "No credentials required for this provider.",
      };
  }
}
