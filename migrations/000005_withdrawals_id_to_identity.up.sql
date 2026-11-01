DO $$
DECLARE old_sequence TEXT;
BEGIN
    SELECT pg_get_serial_sequence('withdrawals', 'id') INTO old_sequence;
    ALTER TABLE withdrawals ALTER COLUMN id DROP DEFAULT;
    EXECUTE format('DROP SEQUENCE %s', old_sequence);
END;
$$;

ALTER TABLE withdrawals ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY;
SELECT setval(
    pg_get_serial_sequence('withdrawals', 'id'),
    COALESCE(MAX(id), 0) + 1,
    false
) FROM withdrawals;
