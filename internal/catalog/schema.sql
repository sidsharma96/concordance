CREATE TABLE concordance_imports (
    collection_id text NOT NULL CHECK (octet_length(collection_id) BETWEEN 1 AND 256),
    import_key text NOT NULL CHECK (octet_length(import_key) BETWEEN 1 AND 256),
    fingerprint_version text NOT NULL CHECK (octet_length(fingerprint_version) BETWEEN 1 AND 64),
    input_fingerprint bytea NOT NULL CHECK (octet_length(input_fingerprint) = 32),
    generation_id text CHECK (octet_length(generation_id) BETWEEN 1 AND 256),
    manifest_key text CHECK (octet_length(manifest_key) BETWEEN 1 AND 1024),
    manifest_sha256 bytea,
    manifest_bytes bigint,
    PRIMARY KEY (collection_id, import_key),
    CONSTRAINT concordance_imports_generation_unique UNIQUE (collection_id, generation_id),
    CONSTRAINT concordance_imports_complete_reference CHECK (
        (generation_id IS NULL AND manifest_key IS NULL AND manifest_sha256 IS NULL AND manifest_bytes IS NULL)
        OR
        (generation_id IS NOT NULL AND manifest_key IS NOT NULL
         AND manifest_sha256 IS NOT NULL AND octet_length(manifest_sha256) = 32
         AND manifest_bytes IS NOT NULL AND manifest_bytes > 0)
    )
);
