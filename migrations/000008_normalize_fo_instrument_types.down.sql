-- Revert F&O instrument_type values back to NSE new-format vendor codes.
BEGIN;

UPDATE instruments SET instrument_type = 'IDF' WHERE instrument_type = 'FUTIDX';
UPDATE instruments SET instrument_type = 'IDO' WHERE instrument_type = 'OPTIDX';
UPDATE instruments SET instrument_type = 'STF' WHERE instrument_type = 'FUTSTK';
UPDATE instruments SET instrument_type = 'STO' WHERE instrument_type = 'OPTSTK';

COMMIT;
