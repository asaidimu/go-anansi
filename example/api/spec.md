# API Documentation

This document provides a comprehensive overview of the API, detailing its structure, endpoints, and data models.

## 1. Overview

The API is built as a RESTful service to interact with a persistence layer. It provides a consistent response pattern for all operations, with proper HTTP status codes and error handling.

### 1.1. Core Concepts

* **Standard HTTP Response**: All responses use appropriate HTTP status codes with consistent JSON structure.
* **Error Handling**: Errors are communicated through HTTP status codes with detailed error information in the response body.
* **Content Negotiation**: All endpoints accept and return `application/json`.
* **Authentication**: All endpoints require authentication via `Authorization: Bearer <token>` header.

### 1.2. Response Format

**Success Response Structure:**
```json
{
  "data": {},
  "meta": {
    "timestamp": "2025-01-15T10:30:00Z",
    "request": "3b822db2-394c-4d7b-8254-c4465296a62f"
  }
}
```

**Error Response Structure:**
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "The request contains invalid data",
    "details": {
      "field": "email",
      "reason": "Invalid email format"
    }
  },
  "meta": {
    "timestamp": "2025-01-15T10:30:00Z",
    "request_id": "req_123abc"
  }
}
```

## 2. API Endpoints

### 2.1. Collection Data Operations

All collection data operations follow the pattern: `/api/collections/{collection}/...`

#### 2.1.1. Create Documents

* **URL**: `POST /api/collections/{collection}/documents`
* **Request Headers**:
  * `Content-Type: application/json`
  * `Authorization: Bearer <token>`
* **Request Body**:
```json
{
  "documents": [
    {
      "id": "optional_custom_id",
      "field1": "value1",
      "field2": "value2"
    }
  ]
}
```
* **Success Response**:
  * **Status**: `201 Created`
  * **Headers**: `Location: /api/collections/{collection}/documents/{id}`
  * **Body**:
```json
{
  "data": {
    "documents": [
      {
        "id": "generated_or_custom_id",
        "field1": "value1",
        "field2": "value2",
        "created_at": "2025-01-15T10:30:00Z",
        "updated_at": "2025-01-15T10:30:00Z",
        "version": 1
      }
    ]
  },
  "meta": {
    "created_count": 1,
    "timestamp": "2025-01-15T10:30:00Z",
    "request_id": "req_123abc"
  }
}
```

#### 2.1.2. Read Documents

* **URL**: `GET /api/collections/{collection}/documents`
* **Query Parameters**:
  * `filter`: JSON-encoded filter object (optional)
  * `sort`: JSON-encoded sort object (optional)
  * `limit`: Number of documents to return (default: 50, max: 1000)
  * `offset`: Number of documents to skip (default: 0)
  * `fields`: Comma-separated list of fields to return (optional)
* **Example**: `GET /api/collections/users/documents?filter={"status":"active"}&limit=10&sort={"created_at":-1}`
* **Success Response**:
  * **Status**: `200 OK`
  * **Body**:
```json
{
  "data": {
    "documents": [
      {
        "id": "doc_123",
        "field1": "value1",
        "created_at": "2025-01-15T10:30:00Z",
        "version": 1
      }
    ]
  },
  "meta": {
    "total_count": 150,
    "returned_count": 10,
    "offset": 0,
    "limit": 10,
    "has_more": true,
    "timestamp": "2025-01-15T10:30:00Z",
    "request_id": "req_123abc"
  }
}
```

#### 2.1.3. Read Single Document

* **URL**: `GET /api/collections/{collection}/documents/{id}`
* **Query Parameters**:
  * `fields`: Comma-separated list of fields to return (optional)
* **Success Response**:
  * **Status**: `200 OK`
  * **Body**:
```json
{
  "data": {
    "id": "doc_123",
    "field1": "value1",
    "field2": "value2",
    "created_at": "2025-01-15T10:30:00Z",
    "updated_at": "2025-01-15T10:30:00Z",
    "version": 3
  },
  "meta": {
    "timestamp": "2025-01-15T10:30:00Z",
    "request_id": "req_123abc"
  }
}
```
* **Error Response**: `404 Not Found` if document doesn't exist

#### 2.1.4. Update Documents

* **URL**: `PATCH /api/collections/{collection}/documents`
* **Request Headers**:
  * `Content-Type: application/json`
  * `If-Match: "version_number"` (optional, for optimistic locking)
* **Request Body**:
```json
{
  "filter": {
    "status": "pending"
  },
  "data": {
    "status": "processed",
    "processed_at": "2025-01-15T10:30:00Z"
  },
  "options": {
    "upsert": false,
    "return_documents": false
  }
}
```
* **Success Response**:
  * **Status**: `200 OK`
  * **Body**:
```json
{
  "data": {
    "updated_count": 5,
    "matched_count": 5,
    "documents": []
  },
  "meta": {
    "timestamp": "2025-01-15T10:30:00Z",
    "request_id": "req_123abc"
  }
}
```

#### 2.1.5. Update Single Document

* **URL**: `PATCH /api/collections/{collection}/documents/{id}`
* **Request Headers**:
  * `If-Match: "version_number"` (optional, for optimistic locking)
* **Request Body**:
```json
{
  "field1": "new_value",
  "field2": "updated_value"
}
```
* **Success Response**:
  * **Status**: `200 OK`
  * **Body**: Updated document
* **Error Responses**:
  * `404 Not Found`: Document doesn't exist
  * `409 Conflict`: Version mismatch (if using optimistic locking)
  * `412 Precondition Failed`: If-Match header condition not met

#### 2.1.6. Replace Single Document

* **URL**: `PUT /api/collections/{collection}/documents/{id}`
* **Request Body**: Complete document object
* **Success Response**: `200 OK` (existing) or `201 Created` (new)

#### 2.1.7. Delete Documents

* **URL**: `DELETE /api/collections/{collection}/documents`
* **Query Parameters**:
  * `filter`: JSON-encoded filter object (required)
  * `confirm`: Must be "true" for safety (required)
* **Example**: `DELETE /api/collections/users/documents?filter={"status":"deleted"}&confirm=true`
* **Success Response**:
  * **Status**: `200 OK`
  * **Body**:
```json
{
  "data": {
    "deleted_count": 3
  },
  "meta": {
    "timestamp": "2025-01-15T10:30:00Z",
    "request_id": "req_123abc"
  }
}
```

#### 2.1.8. Delete Single Document

* **URL**: `DELETE /api/collections/{collection}/documents/{id}`
* **Success Response**: `204 No Content`
* **Error Response**: `404 Not Found` if document doesn't exist

#### 2.1.9. Validate Documents

* **URL**: `POST /api/collections/{collection}/documents/validate`
* **Request Body**:
```json
{
  "documents": [
    {
      "field1": "value1",
      "field2": "value2"
    }
  ],
  "options": {
    "strict": true
  }
}
```
* **Success Response**:
  * **Status**: `200 OK`
  * **Body**:
```json
{
  "data": {
    "valid": true,
    "results": [
      {
        "valid": true,
        "errors": []
      }
    ]
  }
}
```

### 2.2. Collection Management Operations

#### 2.2.1. List Collections

* **URL**: `GET /api/collections`
* **Query Parameters**:
  * `include_schema`: Include schema information (default: false)
  * `limit`: Number of collections to return (optional)
* **Success Response**:
  * **Status**: `200 OK`
  * **Body**:
```json
{
  "data": {
    "collections": [
      {
        "name": "users",
        "document_count": 1500,
        "created_at": "2025-01-01T00:00:00Z",
        "updated_at": "2025-01-15T10:30:00Z"
      },
      {
        "name": "products",
        "document_count": 250,
        "created_at": "2025-01-05T12:00:00Z",
        "updated_at": "2025-01-14T15:20:00Z"
      }
    ]
  },
  "meta": {
    "total_count": 2,
    "timestamp": "2025-01-15T10:30:00Z",
    "request_id": "req_123abc"
  }
}
```

#### 2.2.2. Create Collection

* **URL**: `POST /api/collections`
* **Request Body**:
```json
{
  "schema": {
    "name": "new_collection",
    "version": "1.0.0",
    "fields": {
      "name": {
        "type": "string",
        "required": true,
        "max_length": 100
      },
      "email": {
        "type": "string",
        "required": true,
        "format": "email",
        "unique": true
      }
    },
    "indexes": [
      {
        "fields": ["email"],
        "unique": true
      }
    ]
  }
}
```
* **Success Response**:
  * **Status**: `201 Created`
  * **Headers**: `Location: /api/collections/new_collection`
* **Error Response**: `409 Conflict` if collection already exists

#### 2.2.3. Get Collection Details

* **URL**: `GET /api/collections/{collection}`
* **Success Response**:
  * **Status**: `200 OK`
  * **Body**:
```json
{
  "data": {
    "name": "users",
    "document_count": 1500,
    "schema_version": "1.2.0",
    "created_at": "2025-01-01T00:00:00Z",
    "updated_at": "2025-01-15T10:30:00Z",
    "indexes": [
      {
        "fields": ["email"],
        "unique": true
      }
    ]
  }
}
```

#### 2.2.4. Get Collection Schema

* **URL**: `GET /api/collections/{collection}/schema`
* **Query Parameters**:
  * `version`: Specific schema version (optional)
* **Success Response**:
  * **Status**: `200 OK`
  * **Body**: Complete schema definition

#### 2.2.5. Update Collection Schema

* **URL**: `PUT /api/collections/{collection}/schema`
* **Request Body**: Complete schema definition
* **Success Response**: `200 OK`

#### 2.2.6. Delete Collection

* **URL**: `DELETE /api/collections/{collection}`
* **Query Parameters**:
  * `confirm`: Must be collection name for safety (required)
* **Success Response**: `204 No Content`
* **Error Response**: `400 Bad Request` if confirmation doesn't match

### 2.3. Transaction Operations

TO BE DISCUSSSED

### 2.4. Subscription Operations

#### 2.4.1. List Subscriptions

* **URL**: `GET /api/subscriptions`
* **Query Parameters**:
  * `collection`: Filter by collection (optional)
  * `event_type`: Filter by event type (optional)
* **Success Response**: `200 OK` with array of subscriptions

#### 2.4.2. Create Subscription

* **URL**: `POST /api/subscriptions`
* **Request Body**:
```json
{
  "event_type": "document.created",
  "collection": "users",
  "webhook_url": "https://api.example.com/webhooks/user-created",
  "filters": {
    "status": "active"
  },
  "options": {
    "retry_attempts": 3,
    "retry_delay_ms": 1000
  }
}
```
* **Success Response**:
  * **Status**: `201 Created`
  * **Headers**: `Location: /api/subscriptions/{subscription_id}`

#### 2.4.3. Get Subscription

* **URL**: `GET /api/subscriptions/{subscription_id}`
* **Success Response**: `200 OK` with subscription details

#### 2.4.4. Update Subscription

* **URL**: `PATCH /api/subscriptions/{subscription_id}`
* **Request Body**: Partial subscription object
* **Success Response**: `200 OK`

#### 2.4.5. Delete Subscription

* **URL**: `DELETE /api/subscriptions/{subscription_id}`
* **Success Response**: `204 No Content`

### 2.5. Schema Migration Operations

#### 2.5.1. Create Migration

* **URL**: `POST /api/collections/{collection}/migrations`
* **Request Body**:
```json
{
  "from_version": "1.0.0",
  "to_version": "1.1.0",
  "operations": [
    {
      "type": "add_field",
      "field": "phone",
      "definition": {
        "type": "string",
        "required": false,
        "format": "phone"
      }
    }
  ],
  "options": {
    "dry_run": false,
    "batch_size": 1000
  }
}
```
* **Success Response**: `202 Accepted` with migration job details

#### 2.5.2. Get Migration Status

* **URL**: `GET /api/collections/{collection}/migrations/{migration_id}`
* **Success Response**: `200 OK` with migration status

#### 2.5.3. Rollback Migration

* **URL**: `POST /api/collections/{collection}/migrations/{migration_id}/rollback`
* **Request Body**:
```json
{
  "target_version": "1.0.0",
  "options": {
    "dry_run": false
  }
}
```
* **Success Response**: `202 Accepted`

## 3. HTTP Status Codes

The API uses standard HTTP status codes:

### Success Codes
* `200 OK`: Successful GET, PATCH, DELETE operations
* `201 Created`: Successful POST operations that create resources
* `202 Accepted`: Asynchronous operations that have been queued
* `204 No Content`: Successful DELETE operations

### Client Error Codes
* `400 Bad Request`: Invalid request format or parameters
* `401 Unauthorized`: Missing or invalid authentication
* `403 Forbidden`: Insufficient permissions
* `404 Not Found`: Resource doesn't exist
* `409 Conflict`: Resource conflict (e.g., duplicate key, version mismatch)
* `412 Precondition Failed`: Conditional request failed
* `422 Unprocessable Entity`: Valid request format but semantic errors
* `429 Too Many Requests`: Rate limit exceeded

### Server Error Codes
* `500 Internal Server Error`: Unexpected server error
* `502 Bad Gateway`: Upstream service error
* `503 Service Unavailable`: Service temporarily unavailable
* `504 Gateway Timeout`: Upstream service timeout

## 4. Rate Limiting

All endpoints are rate-limited per authentication token:
* **Default**: 1000 requests per hour
* **Burst**: Up to 100 requests per minute

Rate limit headers are included in all responses:
```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1705320600
X-RateLimit-Retry-After: 3600
```

## 5. Pagination

List endpoints support pagination with consistent parameters:
* `limit`: Number of items per page (default: 50, max: 1000)
* `offset`: Number of items to skip
* `cursor`: Cursor-based pagination token (alternative to offset)

Response includes pagination metadata:
```json
{
  "data": {...},
  "meta": {
    "timestamp": "2025-01-15T10:30:00Z",
    "request": "3b822db2-394c-4d7b-8254-c4465296a62f",
    "pagination": {
      "total_count": 1500,
      "returned_count": 50,
      "offset": 100,
      "limit": 50,
      "has_more": true,
      "next_cursor": "eyJpZCI6IjEyMyJ9"
    }
  }
}
```

## 6. Error Handling

### Error Response Format
All errors return a consistent structure with appropriate HTTP status codes:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Request validation failed",
    "details": {
      "field": "email",
      "reason": "Invalid email format",
      "provided": "invalid-email"
    }
  },
  "meta": {
    "timestamp": "2025-01-15T10:30:00Z",
    "request_id": "req_123abc"
  }
}
```

### Common Error Codes
* `VALIDATION_ERROR`: Request data validation failed
* `AUTHENTICATION_ERROR`: Authentication credentials invalid
* `AUTHORIZATION_ERROR`: Insufficient permissions
* `RESOURCE_NOT_FOUND`: Requested resource doesn't exist
* `RESOURCE_CONFLICT`: Resource state conflict
* `RATE_LIMIT_EXCEEDED`: Too many requests
* `INTERNAL_ERROR`: Unexpected server error
* `SERVICE_UNAVAILABLE`: Service temporarily unavailable

## 7. Authentication & Authorization

To be DISCUSSSED

## 8. Content Types

* **Request Content-Type**: `application/json`
* **Response Content-Type**: `application/json`
* **Character Encoding**: UTF-8

## 9. Versioning

API version is specified in the URL path: `/api/v1/...`
* Current version: `blank`
* Specific version: `v8`
* Version deprecation notices included in response headers when applicable
