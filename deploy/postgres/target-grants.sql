-- Apply only after the target table exists, as its owner/administrator.
-- Restores using --no-acl need these grants separately from the base catalog grants.
GRANT SELECT, INSERT ON catalog.concordance_search_targets TO concordance_app;
GRANT UPDATE (collection_id, generation_id, default_edition, revision)
    ON catalog.concordance_search_targets TO concordance_app;
