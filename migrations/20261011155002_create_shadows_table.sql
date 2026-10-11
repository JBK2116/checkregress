-- +goose Up
CREATE TABLE shadows (
    id               BIGSERIAL   PRIMARY KEY,
    canonical_path   TEXT        NOT NULL,
    legacy_url       TEXT        NOT NULL,
    secondary_url    TEXT        NOT NULL,
    candidate_url    TEXT        NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    status_legacy    SMALLINT    NOT NULL,
    status_candidate SMALLINT    NOT NULL,
    status_match     BOOLEAN     NOT NULL,

    headers          JSONB,
    body             JSONB,

    CONSTRAINT shadows_status_match_ck
        CHECK (status_match = (status_legacy = status_candidate)),
    CONSTRAINT shadows_row_kind_ck
        CHECK (
            (status_match AND headers IS NOT NULL AND body IS NOT NULL) OR
            (NOT status_match AND headers IS NULL AND body IS NULL)
        )
);

CREATE INDEX idx_shadows_path_created
    ON shadows (canonical_path, created_at DESC);

-- +goose Down
DROP TABLE shadows;
