-- name: UpsertModel :exec
INSERT INTO models (id, name, version, description, mode, config_schema, runtime_compat, author)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id, version) DO UPDATE SET
    name          = EXCLUDED.name,
    description   = EXCLUDED.description,
    mode          = EXCLUDED.mode,
    config_schema = EXCLUDED.config_schema,
    runtime_compat = EXCLUDED.runtime_compat,
    author        = EXCLUDED.author;

-- name: ListModels :many
SELECT id, name, version, description, mode, config_schema, runtime_compat, author, created_at
FROM models
ORDER BY id, version;

-- name: GetModel :one
SELECT id, name, version, description, mode, config_schema, runtime_compat, author, created_at
FROM models
WHERE id = $1 AND version = $2;

-- name: ListModelVersions :many
SELECT id, name, version, description, mode, config_schema, runtime_compat, author, created_at
FROM models
WHERE id = $1
ORDER BY version;

-- name: CreateSimulation :one
INSERT INTO simulations (id, model_id, model_version, seed, mode, status, config)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, model_id, model_version, seed, mode, status, config, created_at, updated_at, completed_at;

-- name: GetSimulation :one
SELECT id, model_id, model_version, seed, mode, status, config, created_at, updated_at, completed_at
FROM simulations
WHERE id = $1;

-- name: ListSimulations :many
SELECT id, model_id, model_version, seed, mode, status, config, created_at, updated_at, completed_at
FROM simulations
ORDER BY created_at DESC;

-- name: DeleteSimulation :exec
DELETE FROM simulations WHERE id = $1;

-- name: UpdateSimulationStatus :one
UPDATE simulations
SET status = $2,
    updated_at = now(),
    completed_at = CASE WHEN $2 IN ('completed', 'failed', 'stopped') THEN now() ELSE completed_at END
WHERE id = $1
RETURNING id, model_id, model_version, seed, mode, status, config, created_at, updated_at, completed_at;

-- name: SaveSnapshot :exec
INSERT INTO snapshots (id, simulation_id, schema_version, engine_version, data, checksum)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetSnapshot :one
SELECT id, simulation_id, schema_version, engine_version, data, checksum, created_at
FROM snapshots
WHERE id = $1;

-- name: ListSnapshots :many
SELECT id, simulation_id, schema_version, engine_version, data, checksum, created_at
FROM snapshots
WHERE simulation_id = $1
ORDER BY created_at DESC;
