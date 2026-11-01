ALTER TABLE withdrawals ALTER COLUMN id DROP IDENTITY;
CREATE SEQUENCE withdrawals_id_seq as BIGINT OWNED BY withdrawals.id;

ALTER TABLE withdrawals ALTER COLUMN id SET DEFAULT nextval('withdrawals_id_seq');

SELECT setval(
   'withdrawals_id_seq', COALESCE(MAX(id), 0) + 1, false
) FROM withdrawals;
