-- ============================================================
-- 정기점검 데이터 이관 (localhost → 운영 서버)
-- 생성: 2026-08-14 · 원본: server/data/app.db
-- 계획 1건 · 방문 76건 · 점검사이트 설정 67건
--
-- 실행 전 반드시 서버 DB를 백업하세요:
--   cp data/app.db data/app.db.bak-$(date +%Y%m%d-%H%M%S)
--
-- 실행:
--   sqlite3 data/app.db < 정기점검_이관.sql
-- ============================================================

BEGIN TRANSACTION;

-- ── 사전 점검 ────────────────────────────────────────────────
-- 아래 두 값이 0이면 고객 데이터가 없는 것입니다.
-- 그 상태로 넣으면 방문은 들어가지만 화면에 기관명이 안 나옵니다.
SELECT '고객 수: ' || COUNT(*) FROM customers;
SELECT '기존 방문 수: ' || COUNT(*) FROM maintenance_visits;

-- ── 1. 연도 계획 ─────────────────────────────────────────────
INSERT OR IGNORE INTO maintenance_plans
  (plan_id, plan_year, title, status, created_at, updated_at)
  VALUES ('mpl_a45f8603d9b3d8ac', 2026, '2026년 정기점검', 'draft', '2026-08-05 06:42:13', '2026-08-12 08:43:37');

-- ── 2. 방문 (76건) ───────────────────────────────────────────
-- visit_id 가 이미 있으면 건너뜁니다(INSERT OR IGNORE).
-- project_id · data_origin 컬럼이 서버에 없으면 003 마이그레이션을 먼저 적용하세요.
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_826626a39aefe460', 'mpl_a45f8603d9b3d8ac', '2026-08-03', 'C041-26-006', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-03');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_6387a52a3d81578f', 'mpl_a45f8603d9b3d8ac', '2026-08-03', 'C041-26-013', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-03');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_589ae11e1e5f4193', 'mpl_a45f8603d9b3d8ac', '2026-08-03', 'C041-26-071', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-03');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_33eec6fed2d29a9a', 'mpl_a45f8603d9b3d8ac', '2026-08-03', 'C044-26-001', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-03');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_fa3e90ba7475f517', 'mpl_a45f8603d9b3d8ac', '2026-08-03', 'C044-26-009', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-03');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_b35fc9b0cc5c5ba1', 'mpl_a45f8603d9b3d8ac', '2026-08-03', 'C044-26-026', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-03');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_93708cd8170e7507', 'mpl_a45f8603d9b3d8ac', '2026-08-03', 'C044-26-031', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-03');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_97d18a1dd4c524ad', 'mpl_a45f8603d9b3d8ac', '2026-08-04', 'C041-26-003', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-04');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_03bff0ee408abbf6', 'mpl_a45f8603d9b3d8ac', '2026-08-04', 'C041-26-009', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-04');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_bcc50653d082ce31', 'mpl_a45f8603d9b3d8ac', '2026-08-04', 'C041-26-015', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-04');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_7f4f79725cc455c9', 'mpl_a45f8603d9b3d8ac', '2026-08-04', 'C041-26-016', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-04');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_14960632a47e640d', 'mpl_a45f8603d9b3d8ac', '2026-08-04', 'C041-26-044', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-04');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_e804077bcc7442f7', 'mpl_a45f8603d9b3d8ac', '2026-08-04', 'C041-26-064', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-04');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_d681df45959f6585', 'mpl_a45f8603d9b3d8ac', '2026-08-05', 'C044-26-002', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-05');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_1901db5329266465', 'mpl_a45f8603d9b3d8ac', '2026-08-05', 'C044-26-022', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-05');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_26f918d5dfbfd5b8', 'mpl_a45f8603d9b3d8ac', '2026-08-06', 'C041-26-011', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-06');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_a41e8759b5a319c9', 'mpl_a45f8603d9b3d8ac', '2026-08-06', 'C041-26-012', 0, 0, 'normal', NULL, '2026-08-05 08:00:20', '최혜영', 'KLAS', 1, '2026-08-06');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_e394ca53f52dadf3', 'mpl_a45f8603d9b3d8ac', '2026-08-06', 'C041-26-065', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-06');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_306e4a0014e1b3db', 'mpl_a45f8603d9b3d8ac', '2026-08-07', 'C041-26-031', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-07');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_31f8043ed6189256', 'mpl_a45f8603d9b3d8ac', '2026-08-07', 'C041-26-049', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-07');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_e33a5fd6f481ff2c', 'mpl_a45f8603d9b3d8ac', '2026-08-07', 'C044-26-005', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-07');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_05c7aba4a1d44d79', 'mpl_a45f8603d9b3d8ac', '2026-08-07', 'C044-26-006', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-07');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_753b94b736964ec8', 'mpl_a45f8603d9b3d8ac', '2026-08-07', 'C044-26-008', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-07');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_8778be5cf444ca6e', 'mpl_a45f8603d9b3d8ac', '2026-08-07', 'C044-26-013', 0, 0, 'normal', '임예지주무관님 휴직(3명의 주무관님이 돌아가면서 오전보심)', '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-07');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_1afe226af4bdb4b4', 'mpl_a45f8603d9b3d8ac', '2026-08-07', 'C044-26-032', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-07');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_08fa16e61a3bd77d', 'mpl_a45f8603d9b3d8ac', '2026-08-10', 'C041-26-021', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-10');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_86970163103beb75', 'mpl_a45f8603d9b3d8ac', '2026-08-10', 'C041-26-046', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-10');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_ed7418d1c728d188', 'mpl_a45f8603d9b3d8ac', '2026-08-10', 'C041-26-048', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-10');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_3b16f571990b9028', 'mpl_a45f8603d9b3d8ac', '2026-08-10', 'C041-26-055', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-10');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_e82594c95e8e7736', 'mpl_a45f8603d9b3d8ac', '2026-08-10', 'C041-26-060', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-10');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_9346e52c42d10575', 'mpl_a45f8603d9b3d8ac', '2026-08-10', 'C041-26-070', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-10');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_6ed2e42cc67e55d1', 'mpl_a45f8603d9b3d8ac', '2026-08-11', 'C041-26-005', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-11');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_408cb7c22bf68b20', 'mpl_a45f8603d9b3d8ac', '2026-08-11', 'C041-26-007', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-11');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_0a5e5a78959a441f', 'mpl_a45f8603d9b3d8ac', '2026-08-11', 'C041-26-010', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-11');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_3cc34a656eba3141', 'mpl_a45f8603d9b3d8ac', '2026-08-11', 'C041-26-014', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-11');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_956716512c46d828', 'mpl_a45f8603d9b3d8ac', '2026-08-11', 'C041-26-029', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 1, '2026-08-11');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_01cb68137f659d07', 'mpl_a45f8603d9b3d8ac', '2026-08-12', 'C041-26-030', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_6363f66dc45d188a', 'mpl_a45f8603d9b3d8ac', '2026-08-12', 'C041-26-040', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_e343bde09eee6eb2', 'mpl_a45f8603d9b3d8ac', '2026-08-12', 'C041-26-047', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_a4ab7b50cc7b09ac', 'mpl_a45f8603d9b3d8ac', '2026-08-12', 'C044-26-012', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_b2170e7206c881a8', 'mpl_a45f8603d9b3d8ac', '2026-08-12', 'C044-26-015', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_cb1789a6307a05f3', 'mpl_a45f8603d9b3d8ac', '2026-08-13', 'C041-26-001', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_ab488984ec81f940', 'mpl_a45f8603d9b3d8ac', '2026-08-13', 'C041-26-002', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_fd8f9b23773ef2a0', 'mpl_a45f8603d9b3d8ac', '2026-08-13', 'C041-26-008', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_19d746e16e051c99', 'mpl_a45f8603d9b3d8ac', '2026-08-13', 'C041-26-017', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_a222070b6c81697c', 'mpl_a45f8603d9b3d8ac', '2026-08-13', 'C041-26-020', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_0a29accd193f65db', 'mpl_a45f8603d9b3d8ac', '2026-08-14', 'C044-26-003', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-14');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_bd2f851e6628d1fb', 'mpl_a45f8603d9b3d8ac', '2026-08-14', 'C044-26-007', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 1, '2026-08-14');
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_12721a4f7fd5bb2b', 'mpl_a45f8603d9b3d8ac', '2026-08-14', 'C044-26-016', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_f7ff95455a7c2da1', 'mpl_a45f8603d9b3d8ac', '2026-08-18', 'C044-26-010', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_09f89e2d86053830', 'mpl_a45f8603d9b3d8ac', '2026-08-18', 'C044-26-015', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_a60ab98473c16b7c', 'mpl_a45f8603d9b3d8ac', '2026-08-18', 'C044-26-017', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_1884dd88da7ac572', 'mpl_a45f8603d9b3d8ac', '2026-08-18', 'C044-26-018', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_7245b984565fe6b3', 'mpl_a45f8603d9b3d8ac', '2026-08-18', 'C044-26-019', 0, 0, 'normal', NULL, '2026-08-05 06:44:48', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_c8be4d38a4b5b05a', 'mpl_a45f8603d9b3d8ac', '2026-08-18', 'C044-26-020', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_d30a7697d42c3344', 'mpl_a45f8603d9b3d8ac', '2026-08-18', 'C044-26-020', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_f69505918e2d0bf8', 'mpl_a45f8603d9b3d8ac', '2026-08-18', 'C044-26-021', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_1dc71962922a31ff', 'mpl_a45f8603d9b3d8ac', '2026-08-18', 'C044-26-023', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_f4712cff0a302530', 'mpl_a45f8603d9b3d8ac', '2026-08-18', 'C044-26-032', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_786bd3ca2ed5c3d7', 'mpl_a45f8603d9b3d8ac', '2026-08-19', 'C044-26-004', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_75367f70c964f6b1', 'mpl_a45f8603d9b3d8ac', '2026-08-19', 'C044-26-008', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_39afc6b5bff75911', 'mpl_a45f8603d9b3d8ac', '2026-08-19', 'C044-26-010', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_cac4f60e1724225a', 'mpl_a45f8603d9b3d8ac', '2026-08-19', 'C044-26-013', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_b4df68bfe8d37a66', 'mpl_a45f8603d9b3d8ac', '2026-08-19', 'C044-26-022', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_ee0ec85447ce0d16', 'mpl_a45f8603d9b3d8ac', '2026-08-20', 'C000-26-006', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_37e781280efa9005', 'mpl_a45f8603d9b3d8ac', '2026-08-20', 'C041-26-004', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_90b38b0dcb3250f7', 'mpl_a45f8603d9b3d8ac', '2026-08-20', 'C041-26-005', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_e03b5dbfba7f62b1', 'mpl_a45f8603d9b3d8ac', '2026-08-20', 'C041-26-018', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_c6c6c72834f0863a', 'mpl_a45f8603d9b3d8ac', '2026-08-20', 'C041-26-019', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_9d3b87d2a4a3ad78', 'mpl_a45f8603d9b3d8ac', '2026-08-20', 'C044-26-002', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_3b825b06999fa125', 'mpl_a45f8603d9b3d8ac', '2026-08-21', 'C044-26-001', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_bbce598215503623', 'mpl_a45f8603d9b3d8ac', '2026-08-21', 'C044-26-009', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_80929380a7dbd681', 'mpl_a45f8603d9b3d8ac', '2026-08-21', 'C044-26-011', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_8ea412fa3632f537', 'mpl_a45f8603d9b3d8ac', '2026-08-21', 'C044-26-014', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_3330ce5b493cd300', 'mpl_a45f8603d9b3d8ac', '2026-08-21', 'C044-26-025', 0, 0, 'normal', NULL, '2026-08-05 06:42:13', '최혜영', 'KLAS', 0, NULL);
INSERT OR IGNORE INTO maintenance_visits
  (visit_id, plan_id, visit_date, customer_id, sort_order, auto_generated, entry_category, notes, created_at, assignee, product_type, completed, completed_date)
  VALUES ('mvs_c8838b4b4687990f', 'mpl_a45f8603d9b3d8ac', '2026-08-24', 'C044-26-027', 0, 0, 'fixed', NULL, '2026-08-05 06:42:13', '양기헌', '앤로보틱스', 0, NULL);

-- ── 3. 점검 사이트 설정 (67건) ───────────────────────────────
-- 서버에 이미 설정이 있으면 이 구간을 통째로 주석 처리하세요.
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C000-26-002', '법제처', '세종', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'quarterly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C000-26-006', '세종중앙공원스마트도서관', '세종', 0, 1, 'normal', NULL, '2026-08-05 01:59:15', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-001', '단국대학교율곡기념도서관', '충남', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-002', '한국기술교육대학교다산정보관', '충남', 0, 1, 'normal', NULL, '2026-08-05 01:54:01', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-003', '공주도서관', '충남', 1, 0, 'normal', NULL, '2026-08-05 01:54:36', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-004', '금산도서관', '충남', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-005', '충남교육청남부평생교육원', '충남', 1, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-006', '당진도서관', '충남', 1, 0, 'normal', NULL, '2026-08-05 01:41:26', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-007', '보령도서관', '충남', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-008', '충남교육청서부평생교육원', '충남', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-009', '부여도서관', '충남', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-010', '서천도서관', '충남', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-011', '성환도서관', '충남', 1, 0, 'normal', NULL, '2026-08-05 01:43:31', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-012', '아산도서관', '충남', 1, 0, 'normal', NULL, '2026-08-05 01:50:23', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-013', '예산도서관', '충남', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-014', '웅천도서관', '충남', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-015', '유구도서관', '충남', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-016', '청양도서관', '충남', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-017', '태안도서관', '충남', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-018', '충남교육청평생교육원', '충남', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-019', '충남교육청학생교육문화원', '충남', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-020', '해미도서관', '충남', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-021', '홍성도서관', '충남', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-024', '국방대학교', '충남', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'odd_bimonthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-029', '논산열린도서관', '충남', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-030', '당진중앙도서관', '충남', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-031', '도솔도서관', '충남', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-040', '서산시립도서관', '충남', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'even_bimonthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-044', '성거도서관', '충남', 0, 1, 'normal', NULL, '2026-08-05 06:25:46', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-046', '순천향대학교중앙도서관', '충남', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-047', '신성대학교', '충남', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-048', '신성대학교기숙사', '충남', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'quarterly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-049', '쌍용도서관', '충남', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-055', '아산시중앙도서관', '충남', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-060', '아산은행나무길스마트도서관', '충남', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-064', '직산도서관', '충남', 0, 1, 'normal', NULL, '2026-08-05 06:25:52', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-065', '천안중앙도서관', '충남', 0, 1, 'normal', NULL, '2026-08-05 06:26:01', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-070', '충교행정', '충남', 1, 0, 'normal', NULL, '2026-07-27 03:53:47', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C041-26-071', '꿀벌도서관', '충남', 1, 0, 'normal', NULL, '2026-08-05 01:55:29', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-001', '소담동도서관', '세종', 1, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-002', '세종시립도서관', '세종', 1, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-003', '고운남측도서관', '세종', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-004', '한솔동도서관', '세종', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-005', '도담동도서관', '세종', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-006', '아름동도서관', '세종', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-007', '종촌동도서관', '세종', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-008', '고운동도서관', '세종', 1, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-009', '보람동도서관', '세종', 1, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-010', '새롬동도서관', '세종', 1, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-011', '대평동도서관', '세종', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-012', '다정동도서관', '세종', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-013', '해밀동도서관', '세종', 1, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-014', '반곡동도서관', '세종', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-015', '나성동도서관', '세종', 1, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-016', '장군면작은도서관', '세종', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-017', '소정면작은도서관', '세종', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-018', '조치원어린이도서관', '세종', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-019', '전의도래샘도서관', '세종', 1, 0, 'normal', NULL, '2026-08-05 06:23:53', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-020', '연동면작은도서관', '세종', 1, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-021', '조치원읍도서관', '세종', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-022', '싱싱도서관', '세종', 1, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-023', '전의나무도서관', '세종', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-025', '책문화센터', '세종', 1, 0, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-026', 'KDI국제정책대학원', '세종', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-027', '국립세종도서관', '세종', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-031', '홍익대학교세종캠퍼스도서관', '세종', 0, 1, 'normal', NULL, '2026-08-04 08:44:10', 'monthly');
INSERT OR IGNORE INTO maintenance_site_config
  (customer_id, short_name, region, has_klas, has_rfid, entry_category, fixed_rule, updated_at, inspection_cycle)
  VALUES ('C044-26-032', '어진동행정복합커뮤니티센터', '세종', 1, 1, 'normal', NULL, '2026-08-05 01:56:51', 'monthly');

-- ── 4. 결과 확인 ─────────────────────────────────────────────
SELECT '계획: ' || COUNT(*) FROM maintenance_plans;
SELECT '방문: ' || COUNT(*) FROM maintenance_visits;
SELECT '  ├ 완료: ' || COUNT(*) FROM maintenance_visits WHERE completed = 1;
SELECT '  └ 예정: ' || COUNT(*) FROM maintenance_visits WHERE COALESCE(completed,0) = 0;
SELECT '사이트설정: ' || COUNT(*) FROM maintenance_site_config;

-- 기관명이 안 붙는 방문이 있는지 (0이어야 정상)
SELECT '고객 미매칭 방문: ' || COUNT(*)
  FROM maintenance_visits v
  LEFT JOIN customers c ON c.customer_id = v.customer_id
 WHERE c.customer_id IS NULL;

COMMIT;

-- 문제가 있으면 COMMIT 대신 ROLLBACK; 으로 바꿔 실행하세요.
