mod handlers;
mod keenetic;
mod models;
mod service;

use axum::Router;
use axum::routing::{get, head};
use clap::Parser;
use metrics_exporter_prometheus::PrometheusBuilder;
use tokio::net::TcpListener;
use tokio::signal;
use tower_http::cors::CorsLayer;
use tracing::info;

#[derive(Parser)]
struct Args {
    /// HTTP server listen address
    #[arg(long, default_value = "0.0.0.0:5332")]
    addr: String,

    /// Enable debug logging
    #[arg(long)]
    debug: bool,
}

#[tokio::main]
async fn main() {
    let args = Args::parse();

    let level = if args.debug { "debug" } else { "info" };
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env().unwrap_or_else(|_| level.into()),
        )
        .init();

    let recorder = PrometheusBuilder::new()
        .install_recorder()
        .expect("failed to install Prometheus recorder");
    let process_collector = metrics_process::Collector::default();
    process_collector.describe();
    process_collector.collect();

    let app = Router::new()
        .route(
            "/api/v1/rules",
            head(handlers::has_rule).post(handlers::add_rule),
        )
        .route(
            "/api/v1/routes",
            get(handlers::list_routes)
                .post(handlers::add_route)
                .delete(handlers::delete_route),
        )
        .route("/api/v1/hosts", get(handlers::list_hosts))
        .route(
            "/metrics",
            get(move || std::future::ready(recorder.render())),
        )
        .layer(CorsLayer::permissive());

    let listener = TcpListener::bind(&args.addr)
        .await
        .expect("failed to bind address");

    info!(addr = %args.addr, "server starting...");

    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown_signal())
        .await
        .expect("server error");
}

async fn shutdown_signal() {
    let ctrl_c = async {
        signal::ctrl_c()
            .await
            .expect("failed to install Ctrl+C handler");
    };

    #[cfg(unix)]
    let terminate = async {
        signal::unix::signal(signal::unix::SignalKind::terminate())
            .expect("failed to install SIGTERM handler")
            .recv()
            .await;
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }

    info!("shutting down server...");
}
