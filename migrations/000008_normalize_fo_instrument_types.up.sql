-- Remap F&O instrument_type values from NSE vendor codes to our internal constants.
-- The new bhavcopy format (FinInstrmTp column) uses IDF/IDO/STF/STO; the legacy
-- format used FUTIDX/OPTIDX/FUTSTK/OPTSTK. We own the stored values — vendor format
-- changes must never propagate into the DB.
BEGIN;

UPDATE instruments SET instrument_type = 'FUTIDX' WHERE instrument_type = 'IDF';
UPDATE instruments SET instrument_type = 'OPTIDX' WHERE instrument_type = 'IDO';
UPDATE instruments SET instrument_type = 'FUTSTK' WHERE instrument_type = 'STF';
UPDATE instruments SET instrument_type = 'OPTSTK' WHERE instrument_type = 'STO';

COMMIT;
