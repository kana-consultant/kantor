-- The email_deliveries CHECKs stay widened: narrowing them would fail on, or
-- (as NOT VALID) later block updates of, deliveries of legal documents. The
-- older code never writes 'legal' or 'party', so the wider CHECK is harmless
-- to it, and the up migration drops and re-adds both constraints.
DROP TABLE IF EXISTS legal_documents;
DROP TABLE IF EXISTS legal_parties;
