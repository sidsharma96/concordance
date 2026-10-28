CREATE TABLE concordance_object_namespaces (
 namespace_id text PRIMARY KEY CHECK (octet_length(namespace_id) BETWEEN 1 AND 256),
 endpoint text NOT NULL CHECK (octet_length(endpoint) BETWEEN 1 AND 2048),
 bucket text NOT NULL CHECK (octet_length(bucket) BETWEEN 1 AND 63),
 collection_id text NOT NULL CHECK (octet_length(collection_id) BETWEEN 1 AND 256),
 key_version text NOT NULL CHECK (key_version='v1'),
 epoch bigint NOT NULL CHECK (epoch>0),
 active boolean NOT NULL,
 UNIQUE(endpoint,bucket,collection_id,key_version)
);
CREATE TABLE concordance_object_receipts (
 namespace_id text NOT NULL REFERENCES concordance_object_namespaces(namespace_id),
 epoch bigint NOT NULL CHECK (epoch>0),
 sha256 bytea NOT NULL CHECK (octet_length(sha256)=32),
 bytes bigint NOT NULL CHECK (bytes>=0),
 etag text NOT NULL CHECK (octet_length(etag) BETWEEN 2 AND 1024),
 policy text NOT NULL CHECK (policy='sha256-get-etag/v1'),
 PRIMARY KEY(namespace_id,epoch,sha256)
);
