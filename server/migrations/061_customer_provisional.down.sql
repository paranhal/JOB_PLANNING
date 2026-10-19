DROP INDEX IF EXISTS idx_customers_provisional;
UPDATE customers SET is_provisional=0, provisional_by='', provisional_at='';
