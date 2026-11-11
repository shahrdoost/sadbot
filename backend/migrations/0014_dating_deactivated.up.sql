-- self-serve account deactivation («حذف حساب»): soft flag, not a data wipe.
ALTER TABLE dating_profiles ADD COLUMN deactivated_at timestamptz;
