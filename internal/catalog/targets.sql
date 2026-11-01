CREATE TABLE concordance_search_targets (
    name text PRIMARY KEY CHECK (octet_length(name) BETWEEN 1 AND 256),
    collection_id text NOT NULL CHECK (octet_length(collection_id) BETWEEN 1 AND 256),
    generation_id text NOT NULL CHECK (octet_length(generation_id) BETWEEN 1 AND 256),
    default_edition text NOT NULL CHECK (octet_length(default_edition) BETWEEN 1 AND 256),
    revision bigint NOT NULL CHECK (revision > 0),
    CONSTRAINT concordance_search_targets_generation FOREIGN KEY (collection_id, generation_id)
        REFERENCES concordance_imports (collection_id, generation_id)
);
