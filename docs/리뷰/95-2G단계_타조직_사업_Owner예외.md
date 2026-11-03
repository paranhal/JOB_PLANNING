# 95-2G단계 — 타 조직 사업 수주·계약 전환 제한 (Owner 예외)

기준: `커서_작업지시서_95-2G_타조직_사업_수주계약전환_Owner예외.md`  
선행: 95-2E. 실행일: 2026-10-23.  
운영·개발 DB를 코드로 바꾸지 않음. `server/data` 미커밋. 마이그레이션 없음.  
배포 이미지는 후속 **95-18** (`06ea4cc`)이 이 코드를 포함.

## 원인

역할 권한(`CanWriteSales`)만으로 전환·수정 링크와 서버 가드가 열려 타 조직 사업을 URL로 전환할 수 있었다. 대시보드는 조직 필터가 있어도 빈 `org_id` 폴백·시점보기(`sim_role`)로 전 조직이 보일 수 있음.

## 동작

- `canManageSalesProject`: vision_admin / 홈 조직 / `SalesOwnerID` 일치. 이름만 Owner 불허. `view_org_id` 우회 불가.
- 거부: `다른 조직의 사업입니다. 사업 Owner 또는 해당 조직만 변경할 수 있습니다.`
- 내 업무 대시보드 `MyScope`: 홈 조직 **또는** 내가 Owner. 사업 목록 조회는 95-2E 유지. 행 `CanManage`.
- 영업 활동 등록은 타 조직 비Owner도 허용.
- `ListContractUnion` 빈 조직은 OrgAll 폴백 안 함. 전체 조회는 명시적 `OrgAll`.

## 로컬 DB (읽기 전용)

`sales_projects` 36건. Owner 이름만 있고 ID 없는 건 **0**. 데이터 `org_id` 보정은 하지 않음.

## 테스트

`TestSales95_2GCurrentOrgAndEmptyUnionFallback` · `HjleeDashboardHidesOtherOrgUnlessOwner` · `WriteGuardsAndActivityException` · `OrgAdminViewOrgBypassAndVisionCLevel` · `EmptyOrgDashboardAndListRead` · `ListRenderBadgeNoConvertForForeign` · `OwnerPromoteSucceeds`.

## 커밋

`d6acc4e9f0d61e754212df8f2d3e8ff36ad7a918` 당시 VERSION 95-2G.
