use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "snake_case")]
pub enum PlanItemStatus {
    Pending,
    InProgress,
    Completed,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(deny_unknown_fields)]
pub struct PlanItem {
    pub step: String,
    pub status: PlanItemStatus,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(deny_unknown_fields)]
pub struct UpdatePlanPayload {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub explanation: Option<String>,
    pub plan: Vec<PlanItem>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct UpdatePlanResult {
    pub ok: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub explanation: Option<String>,
    pub plan: Vec<PlanItem>,
}

pub fn validate_update_plan_payload(payload: &UpdatePlanPayload) -> Result<(), String> {
    if payload.plan.is_empty() {
        return Err("plan must contain at least one item".to_string());
    }
    let mut in_progress_count = 0usize;
    for item in &payload.plan {
        if item.step.trim().is_empty() {
            return Err("plan item step must not be empty".to_string());
        }
        if matches!(item.status, PlanItemStatus::InProgress) {
            in_progress_count += 1;
        }
    }
    if in_progress_count > 1 {
        return Err("plan may contain at most one in_progress item".to_string());
    }
    Ok(())
}
