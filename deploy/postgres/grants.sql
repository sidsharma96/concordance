-- Connect to the intended Concordance database as its administrator; no other database.
-- A restored clone also needs these object grants after --no-acl restoration.
-- CONNECT is deliberately separate: a restored clone must stay inaccessible.
GRANT USAGE ON SCHEMA catalog TO concordance_app;
GRANT SELECT, INSERT ON catalog.concordance_imports,
    catalog.concordance_object_namespaces,
    catalog.concordance_object_receipts TO concordance_app;
GRANT UPDATE (generation_id, manifest_key, manifest_sha256, manifest_bytes)
    ON catalog.concordance_imports TO concordance_app;
GRANT UPDATE (active, epoch) ON catalog.concordance_object_namespaces TO concordance_app;
