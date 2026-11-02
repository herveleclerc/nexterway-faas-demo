use axum::response::IntoResponse;
use axum::Json;
use serde_json::json;

pub async fn handler() -> impl IntoResponse {
    Json(json!({
        "message": "Hello from Rust on Nexterway FaaS!",
        "status": "healthy",
        "runtime": "rust-axum",
        "framework": "fission-rust",
        "architecture": std::env::consts::ARCH,
        "os": std::env::consts::OS
    }))
}

