use axum::Json;
use axum::extract::Query;
use axum::http::StatusCode;
use axum::response::IntoResponse;
use serde::Deserialize;

use crate::models::*;
use crate::service;

#[derive(Deserialize)]
pub struct HasRuleParams {
    pub table: u32,
    pub iif: String,
}

pub async fn has_rule(Query(params): Query<HasRuleParams>) -> impl IntoResponse {
    match service::has_rule(params.table, &params.iif) {
        Ok(true) => StatusCode::OK.into_response(),
        Ok(false) => StatusCode::NOT_FOUND.into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, Json(e)).into_response(),
    }
}

pub async fn add_rule(Json(rule): Json<Rule>) -> impl IntoResponse {
    match service::add_rule(&rule) {
        Ok(()) => StatusCode::NO_CONTENT.into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, Json(e)).into_response(),
    }
}

#[derive(Deserialize)]
pub struct ListRoutesParams {
    pub table: u32,
}

pub async fn list_routes(Query(params): Query<ListRoutesParams>) -> impl IntoResponse {
    match service::list_routes(params.table) {
        Ok(routes) => Json(routes).into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, Json(e)).into_response(),
    }
}

pub async fn add_route(Json(route): Json<Route>) -> impl IntoResponse {
    match service::add_route(&route) {
        Ok(()) => StatusCode::NO_CONTENT.into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, Json(e)).into_response(),
    }
}

pub async fn delete_route(Json(route): Json<Route>) -> impl IntoResponse {
    match service::delete_route(&route) {
        Ok(()) => StatusCode::NO_CONTENT.into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, Json(e)).into_response(),
    }
}

pub async fn list_hosts() -> impl IntoResponse {
    match service::list_hosts() {
        Ok(hosts) => Json(hosts).into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, Json(e)).into_response(),
    }
}
