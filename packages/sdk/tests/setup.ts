import dotenv from "dotenv";
import path from "path";
import { LLMVault } from "../src/index.js";

dotenv.config({ path: path.resolve(__dirname, "../../../.env") });

const apiKey = process.env.LLM_VAULT_API_KEY;
if (!apiKey) {
  throw new Error("LLM_VAULT_API_KEY is required in .env");
}

export const vault = new LLMVault({
  apiKey,
  baseUrl: "https://api.dev.llmvault.dev",
});
