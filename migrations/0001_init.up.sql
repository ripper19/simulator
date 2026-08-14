CREATE TABLE IF NOT EXISTS models (
    id              TEXT        NOT NULL,
    name            TEXT        NOT NULL,
    version         TEXT        NOT NULL,
    description     TEXT        NOT NULL DEFAULT '',
    mode            TEXT        NOT NULL DEFAULT '',
    config_schema   JSONB       NOT NULL DEFAULT '{}'::jsonb,
    runtime_compat  TEXT        NOT NULL DEFAULT '',
    author          TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, version)
);

CREATE TABLE IF NOT EXISTS simulations (
    id            TEXT        PRIMARY KEY,
    model_id      TEXT        NOT NULL,
    model_version TEXT        NOT NULL,
    seed          BIGINT      NOT NULL,
    mode          TEXT        NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'created',
    config        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at  TIMESTAMPTZ,
    CONSTRAINT fk_simulation_model FOREIGN KEY (model_id, model_version)
        REFERENCES models (id, version)
);

CREATE INDEX IF NOT EXISTS idx_simulations_model ON simulations (model_id, model_version);
CREATE INDEX IF NOT EXISTS idx_simulations_created ON simulations (created_at DESC);

CREATE TABLE IF NOT EXISTS snapshots (
    id             TEXT        PRIMARY KEY,
    simulation_id  TEXT        NOT NULL,
    schema_version INT         NOT NULL,
    engine_version TEXT        NOT NULL,
    data           JSONB       NOT NULL,
    checksum       TEXT        NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_snapshot_simulation FOREIGN KEY (simulation_id)
        REFERENCES simulations (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_snapshots_simulation ON snapshots (simulation_id, created_at DESC);
