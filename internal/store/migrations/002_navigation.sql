ALTER TABLE services ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0 CHECK(pinned IN (0,1));
ALTER TABLE services ADD COLUMN navigation_order INTEGER NOT NULL DEFAULT 2147483647 CHECK(navigation_order>=0);
CREATE INDEX services_navigation ON services(pinned DESC,navigation_order,created_at,id);
