use std::collections::HashMap;
use std::sync::Arc;

use crate::AgentDefinition;

/// Immutable registry of agent definitions (sub-agents only).
/// Built once when config is pushed, shared via Arc.
pub struct AgentDefinitionRegistry {
    sub_agents: HashMap<String, Arc<AgentDefinition>>,
}

impl AgentDefinitionRegistry {
    pub fn from_definition(parent: Arc<AgentDefinition>) -> Self {
        let sub_agents = parent
            .sub_agents
            .iter()
            .map(|(name, sub)| (name.clone(), Arc::new(sub.clone())))
            .collect();
        Self { sub_agents }
    }

    pub fn resolve(&self, name: &str) -> Option<Arc<AgentDefinition>> {
        self.sub_agents.get(name).cloned()
    }

    pub fn available_agents(&self) -> Vec<String> {
        let mut names: Vec<String> = self.sub_agents.keys().cloned().collect();
        names.sort();
        names
    }

    pub fn agent_description(&self, name: &str) -> String {
        match self.resolve(name) {
            Some(def) => {
                let desc = def.agent.description.trim();
                if desc.is_empty() {
                    def.agent.name.clone()
                } else {
                    desc.to_string()
                }
            }
            None => name.to_string(),
        }
    }
}
