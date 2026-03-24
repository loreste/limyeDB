//! LimyeDB Rust Client
//!
//! Official Rust client for LimyeDB - Enterprise Distributed Vector Database.
//!
//! # Example
//!
//! ```rust,no_run
//! use limyedb::{LimyeDBClient, Point, CreateCollectionRequest};
//!
//! #[tokio::main]
//! async fn main() -> Result<(), limyedb::Error> {
//!     let client = LimyeDBClient::new("http://localhost:8080", None)?;
//!
//!     // Create a collection
//!     client.create_collection(CreateCollectionRequest {
//!         name: "documents".to_string(),
//!         dimension: 1536,
//!         metric: Some("cosine".to_string()),
//!         ..Default::default()
//!     }).await?;
//!
//!     // Insert vectors
//!     client.upsert("documents", vec![
//!         Point {
//!             id: "doc1".to_string(),
//!             vector: vec![0.1, 0.2, 0.3],
//!             payload: Some(serde_json::json!({"title": "Hello"})),
//!         }
//!     ]).await?;
//!
//!     // Search
//!     let results = client.search("documents", vec![0.1, 0.2, 0.3], 10, None).await?;
//!     for result in results {
//!         println!("ID: {}, Score: {}", result.id, result.score);
//!     }
//!
//!     Ok(())
//! }
//! ```

use reqwest::{Client, StatusCode};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use thiserror::Error;

/// Error types for LimyeDB client.
#[derive(Error, Debug)]
pub enum Error {
    #[error("HTTP request failed: {0}")]
    Request(#[from] reqwest::Error),

    #[error("JSON serialization failed: {0}")]
    Serialization(#[from] serde_json::Error),

    #[error("Authentication failed")]
    Authentication,

    #[error("Collection not found: {0}")]
    CollectionNotFound(String),

    #[error("Point not found: {0}")]
    PointNotFound(String),

    #[error("Bad request: {0}")]
    BadRequest(String),

    #[error("Server error: {0}")]
    Server(String),
}

/// Result type for LimyeDB operations.
pub type Result<T> = std::result::Result<T, Error>;

/// HNSW index configuration.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct HNSWConfig {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub m: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ef_construction: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ef_search: Option<i32>,
}

/// Request to create a collection.
#[derive(Debug, Clone, Serialize, Default)]
pub struct CreateCollectionRequest {
    pub name: String,
    pub dimension: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub metric: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub hnsw: Option<HNSWConfig>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub on_disk: Option<bool>,
}

/// Collection information.
#[derive(Debug, Clone, Deserialize)]
pub struct Collection {
    pub name: String,
    pub dimension: i32,
    pub metric: String,
    pub points_count: i64,
    pub status: String,
}

/// A vector point.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Point {
    pub id: String,
    pub vector: Vec<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub payload: Option<serde_json::Value>,
}

/// Search result.
#[derive(Debug, Clone, Deserialize)]
pub struct SearchResult {
    pub id: String,
    pub score: f32,
    #[serde(default)]
    pub vector: Option<Vec<f32>>,
    #[serde(default)]
    pub payload: Option<serde_json::Value>,
}

/// Filter condition.
#[derive(Debug, Clone, Serialize)]
pub struct Filter {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub must: Option<Vec<Condition>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub must_not: Option<Vec<Condition>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub should: Option<Vec<Condition>>,
}

/// Filter condition.
#[derive(Debug, Clone, Serialize)]
pub struct Condition {
    pub key: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub r#match: Option<MatchCondition>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub range: Option<RangeCondition>,
}

#[derive(Debug, Clone, Serialize)]
pub struct MatchCondition {
    pub value: serde_json::Value,
}

#[derive(Debug, Clone, Serialize)]
pub struct RangeCondition {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub gt: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub gte: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub lt: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub lte: Option<f64>,
}

/// Search request parameters.
#[derive(Debug, Clone, Serialize, Default)]
pub struct SearchRequest {
    pub vector: Vec<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub limit: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub filter: Option<Filter>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ef: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub with_payload: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub with_vector: Option<bool>,
}

/// LimyeDB client.
#[derive(Clone)]
pub struct LimyeDBClient {
    client: Client,
    base_url: String,
}

impl LimyeDBClient {
    /// Create a new LimyeDB client.
    pub fn new(base_url: &str, api_key: Option<&str>) -> Result<Self> {
        let mut headers = reqwest::header::HeaderMap::new();
        headers.insert(
            reqwest::header::CONTENT_TYPE,
            "application/json".parse().unwrap(),
        );

        if let Some(key) = api_key {
            headers.insert(
                reqwest::header::AUTHORIZATION,
                format!("Bearer {}", key).parse().unwrap(),
            );
        }

        let client = Client::builder()
            .default_headers(headers)
            .timeout(std::time::Duration::from_secs(30))
            .build()?;

        Ok(Self {
            client,
            base_url: base_url.trim_end_matches('/').to_string(),
        })
    }

    /// Create a new collection.
    pub async fn create_collection(&self, request: CreateCollectionRequest) -> Result<Collection> {
        let response = self
            .client
            .post(format!("{}/collections", self.base_url))
            .json(&request)
            .send()
            .await?;

        self.handle_response(response).await
    }

    /// Get collection information.
    pub async fn get_collection(&self, name: &str) -> Result<Collection> {
        let response = self
            .client
            .get(format!("{}/collections/{}", self.base_url, name))
            .send()
            .await?;

        self.handle_response(response).await
    }

    /// List all collections.
    pub async fn list_collections(&self) -> Result<Vec<Collection>> {
        #[derive(Deserialize)]
        struct Response {
            collections: Vec<Collection>,
        }

        let response = self
            .client
            .get(format!("{}/collections", self.base_url))
            .send()
            .await?;

        let data: Response = self.handle_response(response).await?;
        Ok(data.collections)
    }

    /// Delete a collection.
    pub async fn delete_collection(&self, name: &str) -> Result<()> {
        let response = self
            .client
            .delete(format!("{}/collections/{}", self.base_url, name))
            .send()
            .await?;

        if response.status().is_success() {
            Ok(())
        } else {
            Err(self.handle_error(response).await)
        }
    }

    /// Upsert points into a collection.
    pub async fn upsert(&self, collection: &str, points: Vec<Point>) -> Result<UpsertResult> {
        #[derive(Serialize)]
        struct Request {
            points: Vec<Point>,
        }

        let response = self
            .client
            .put(format!("{}/collections/{}/points", self.base_url, collection))
            .json(&Request { points })
            .send()
            .await?;

        self.handle_response(response).await
    }

    /// Get a point by ID.
    pub async fn get_point(&self, collection: &str, point_id: &str) -> Result<Point> {
        let response = self
            .client
            .get(format!(
                "{}/collections/{}/points/{}",
                self.base_url, collection, point_id
            ))
            .send()
            .await?;

        self.handle_response(response).await
    }

    /// Delete points by IDs.
    pub async fn delete_points(&self, collection: &str, ids: Vec<String>) -> Result<DeleteResult> {
        #[derive(Serialize)]
        struct Request {
            ids: Vec<String>,
        }

        let response = self
            .client
            .post(format!(
                "{}/collections/{}/points/delete",
                self.base_url, collection
            ))
            .json(&Request { ids })
            .send()
            .await?;

        self.handle_response(response).await
    }

    /// Search for similar vectors.
    pub async fn search(
        &self,
        collection: &str,
        vector: Vec<f32>,
        limit: i32,
        filter: Option<Filter>,
    ) -> Result<Vec<SearchResult>> {
        let request = SearchRequest {
            vector,
            limit: Some(limit),
            filter,
            with_payload: Some(true),
            ..Default::default()
        };

        self.search_with_params(collection, request).await
    }

    /// Search with full parameters.
    pub async fn search_with_params(
        &self,
        collection: &str,
        request: SearchRequest,
    ) -> Result<Vec<SearchResult>> {
        #[derive(Deserialize)]
        struct Response {
            result: Vec<SearchResult>,
        }

        let response = self
            .client
            .post(format!(
                "{}/collections/{}/search",
                self.base_url, collection
            ))
            .json(&request)
            .send()
            .await?;

        let data: Response = self.handle_response(response).await?;
        Ok(data.result)
    }

    /// Check server health.
    pub async fn health(&self) -> Result<HealthResponse> {
        let response = self
            .client
            .get(format!("{}/health", self.base_url))
            .send()
            .await?;

        self.handle_response(response).await
    }

    async fn handle_response<T: for<'de> Deserialize<'de>>(
        &self,
        response: reqwest::Response,
    ) -> Result<T> {
        let status = response.status();

        if status.is_success() {
            Ok(response.json().await?)
        } else {
            Err(self.handle_error_with_status(status, response).await)
        }
    }

    async fn handle_error(&self, response: reqwest::Response) -> Error {
        self.handle_error_with_status(response.status(), response)
            .await
    }

    async fn handle_error_with_status(
        &self,
        status: StatusCode,
        response: reqwest::Response,
    ) -> Error {
        let text = response.text().await.unwrap_or_default();

        match status {
            StatusCode::UNAUTHORIZED => Error::Authentication,
            StatusCode::NOT_FOUND => {
                if text.to_lowercase().contains("collection") {
                    Error::CollectionNotFound(text)
                } else {
                    Error::PointNotFound(text)
                }
            }
            StatusCode::BAD_REQUEST => Error::BadRequest(text),
            _ => Error::Server(text),
        }
    }
}

/// Upsert operation result.
#[derive(Debug, Deserialize)]
pub struct UpsertResult {
    pub succeeded: i32,
    pub failed: i32,
}

/// Delete operation result.
#[derive(Debug, Deserialize)]
pub struct DeleteResult {
    pub deleted: i32,
}

/// Health check response.
#[derive(Debug, Deserialize)]
pub struct HealthResponse {
    pub status: String,
    pub version: String,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_create_client() {
        let client = LimyeDBClient::new("http://localhost:8080", None);
        assert!(client.is_ok());
    }

    #[test]
    fn test_create_client_with_api_key() {
        let client = LimyeDBClient::new("http://localhost:8080", Some("test-key"));
        assert!(client.is_ok());
    }
}
