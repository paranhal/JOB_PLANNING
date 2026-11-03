# 커서 작업지시서 95-2G — 타 조직 사업: 수주·계약 전환 제한 + 내 업무 대시보드 노출 제한 (Owner 예외)

**상태: 구현 완료 (2026-10-23).** 데이터(`org_id`·Owner) 보정 없음. 마이그레이션 없음. `server/data` 미커밋.

| 항목 | 값 |
|------|-----|
| 커밋 | `d6acc4e9f0d61e754212df8f2d3e8ff36ad7a918` (2026-10-23 15:40 +0900) |
| 당시 버전 | `95-2G` |
| 후속 배포 이미지 | **95-18** `06ea4cc`(이 코드 포함). 운영 `update.sh` 는 sudo 대기일 수 있음 |
| 마이그레이션 | 없음 |
| CSS | `npm run build:css` 같은 커밋(`tailwind.css` 1줄) |

보고서: `docs/리뷰/95-2G단계_타조직_사업_Owner예외.md`

---

## 구현 결과

### 1단계 원인 (재현)

| 질문 | 결과 |
|------|------|
| hjlee `currentOrg` | JWT `org_id` = 홈 조직(도서관 `O01`). `sim_*` 없으면 `OrgAll` 아님 |
| vsadmin 시점보기 | `sim_role=vision_admin` 이면 `currentOrg=OrgAll` → 전 조직이 보일 수 있음 |
| 링크 | `CanWriteSales`(역할)만 봄. 조직·Owner 없음 |
| 서버 | `Edit/Update/Promote/ContractDialog/ChangeStage` 도 역할만 → URL로 타 조직 전환 가능 |
| 빈 `org_id` 폴백 | `listUnlinkedContractedSales` 가 `OrgAll` 로 넘어가 대시보드에 전 조직 미전환이 붙을 수 있음 |
| 데이터 | 코드와 별개로, 업로드·이관 건이 전부 `O01` 이면 조직 필터만으로는 SI/미디어가 안 가려짐. **코드로 보정하지 않음** |

### 2단계 권한 (`handler/sales_manage.go`)

`canManageSalesProject` (UI·서버 동일):

1. `isReadOnly` (C-Level 등) → false  
2. `vision_admin` → true  
3. `canWriteSalesDeal` 아니면 false (활동 권한 `canWriteSales` 와 분리 — 지시서의 `canWriteSales` 보다 사업 상태 변경에 맞춤)  
4. `p.OrgID == currentUserHomeOrg(c)` → true (`view_org_id`·sim 이 홈이 아님)  
5. `uid != "" && uid == p.SalesOwnerID` → true (타 조직 Owner)  
6. 그 외 false. **이름(`SalesOwner`) 일치는 쓰지 않음**

`loadManagedSales` 가 Edit/Update/ChangeStage/Promote*/ContractDialog/Drop/파이프라인 액션/견적·주문 중 사업 상태를 바꾸는 경로에 403.  
거부 문구: `다른 조직의 사업입니다. 사업 Owner 또는 해당 조직만 변경할 수 있습니다.` (HTML은 95-12 flash+303 가능).  
예외: `POST .../activities` 는 `loadManagedSales` 를 타지 않음.

### 3단계 대시보드

- `SalesListFilter.MyScope` + `OwnerUserID`. SQL: `org_id=? OR sales_owner_id=?`. 빈 조직·빈 uid 면 `1=0` (전체 폴백 없음).
- 영업 대시보드 활성/종료/휴면, 파이프라인, 계약 합집합(`salesContractUnionFilter`)에 MyScope.
- vision_admin·C-Level 대시보드는 기존 `OrgAll`/가시 범위. C-Level 전환 링크는 `canManage` false 로 숨김.
- `/sales` 목록은 `applySalesListReadScope` → 내부 직원 `OrgAll`(95-2E). 행 `CanManage`·`ForeignOrg`·뱃지·툴팁 `타 조직 사업 · Owner만 변경`.

### 4단계 템플릿

`sales/dashboard.html` · `project/dashboard.html` · `sales/pipeline.html` · `sales/show.html` · `sales/modal.html` · `sales/list.html` · `contracts/list.html` — 전환/수정은 `.CanManage`.

### 변경 파일

`sales_manage.go`(신규) · `sales.go` · `sales_promote.go` · `sales_pipeline_action.go` · `sales_stage4.go` · `sales_q.go` · `sales_pipeline_xlsx.go` · `project.go` · `project_dashboard.go` · `model/sales.go` · `model/contract_union.go` · `repository/sales_repo.go` · `sales_stage4_repo.go` · `contract_union.go` · 위 템플릿 · `sales_95_2g_http_test.go` · `contract_union_test.go` · `VERSION` 당시 95-2G.

### 테스트 대응

| # | 요구 | 테스트 | 결과 |
|---|------|--------|------|
| 1 | hjlee 대시보드에 P_B 없음 | `TestSales95_2GHjleeDashboardHidesOtherOrgUnlessOwner` | 통과 |
| 2 | Owner면 노출·전환·promote | 위 + `TestSales95_2GOwnerPromoteSucceeds` | 통과 |
| 3 | 비Owner 직접 URL 403 | `TestSales95_2GWriteGuardsAndActivityException` | 통과 |
| 4 | 활동 등록 허용 | 동일 | 통과 |
| 5 | 홈 조직 P_A 수정 가능 | 동일 | 통과 |
| 6 | org_admin `view_org_id` 우회 불가, vsadmin 허용, C-Level 조회만 | `TestSales95_2GOrgAdminViewOrgBypassAndVisionCLevel` | 통과 |
| 7 | 빈 org 대시보드 비노출, 목록은 조회 | `TestSales95_2GEmptyOrgDashboardAndListRead` | 통과 |
| 8 | 빈 OrgID union 폴백 금지, OrgAll 은 유지 | `TestSales95_2GCurrentOrgAndEmptyUnionFallback` | 통과 |
| 9 | 뱃지·툴팁, 파이프라인 전환 버튼 없음 | `TestSales95_2GListRenderBadgeNoConvertForForeign` | 통과 |
| 10 | go test 신규 실패 0, ui_lint | 95-2G 테스트 이름 실패 목록에 없음 | 통과 |

### 레거시 Owner (이름만, ID 없음)

로컬 `server/data/app.db` 읽기: 영업 36건 중 **0건**. 운영은 코드로 세지 않음(데이터 변경 금지). 이름 일치는 계속 안 씀.

### `go test ./...` vs 72A2

95-2G 신규 실패 이름 **0**. leftover는 72A2 + 95-13~16 계열(`TestUILintRatchet` 등). 이번 템플릿은 뱃지·기존 유틸, 원색 추가 없음.

### 남은 이슈

운영에서 SI/미디어 사업이 `org_id=O01`(도서관)으로 들어가 있으면 **데이터**. 화면에는 홈 조직으로 보인다. 보정은 사용자가 운영에서 별도. 엑셀 업로드가 org를 빼먹는지는 95-2 쪽 후속.

---


## 요구 (사용자 확정)
도서관사업팀 영업 담당(hjlee)으로 로그인했는데 SI사업팀·미디어사업팀 사업이 대시보드에 보이고 `계약으로 전환` 링크가 나온다.
- **타 조직이라도 사업 Owner(`sales_owner_id` = 로그인 사용자)이면**: 대시보드에 표시, 수정, 수주 전환, 계약 전환 모두 가능.
- **타 조직이고 Owner도 아니면**: **영업 활동 등록(활동 로그)만 가능**. 수주 전환·계약 전환·수정·단계 변경은 불가. **내 업무 대시보드에도 나타나지 않는다.**
- 사업 **조회**(사업 목록/상세 읽기)는 95-2E 정책대로 전 직원 허용 유지. 이 지시서는 **쓰기 권한 + 대시보드 노출**만 다룬다.

## 현행 코드 확인 결과 (Claude 정독)
- `SalesHandler.Dashboard`(sales.go ~141): `listF.OrgID = currentOrg(c)`로 조직 필터를 거는 구조인데도 타 조직 건이 보임 → 아래 **1단계 원인 재현**이 먼저 필요.
- 미전환 목록: `ListContractUnion(OrgID)` → `listUnlinkedContractedSales`가 `appendOrg(s.org_id)` 사용. org가 비어 있으면 `OrgAll`로 폴백(contract_union.go 160~163), `orgIDOrAll`도 동일 → 조직이 "" 이면 **전 조직 노출**.
- 전환 링크 노출 조건이 `$.CanWriteSales`(역할 권한)뿐이라 **조직·Owner 판정이 없음**: `sales/dashboard.html:17`, `project/dashboard.html:127`, `sales/pipeline.html:89-91`, `sales/show.html:34,536`, `sales/modal.html:15`, `contracts/list.html:105`.
- 서버 가드도 역할 권한만: `canWriteSales`/`canWriteSalesDeal`(rbac.go 133~148). `Edit/Update/PromoteForm/PromoteCustomer/PromoteSave/ContractDialog/ChangeStage` 모두 대상 사업의 조직·Owner를 보지 않음. 즉 **URL 직접 접근으로도 타 조직 사업을 전환 가능**.
- 참고: `canDropSales`(sales_stage4.go:163)는 이미 admin 또는 Owner만 허용 — 같은 패턴을 확장.

## 1단계 — 원인 재현 (코드 수정 전, 보고 필수)
hjlee 노출 경로를 테스트로 먼저 재현하고 아래를 보고한다.
1. hjlee의 `currentOrg(c)` 값(`org_id` 컨텍스트, `sim_role`/`view_org_id` 영향 여부). 사용자가 vsadmin의 "누구 시점으로 보기"로 확인했을 가능성도 있으므로 `sim_*` 경로가 `OrgAll`을 반환하는 조건을 확인.
2. 노출된 사업의 `sales_projects.org_id` 값(비어 있거나 도서관으로 저장되었는지). 95-2 엑셀 업로드·이관 시 org가 누락되었을 가능성.
3. 노출 경로가 `Unpromoted` 외에 `pipeline`, `migrated`, 프로젝트 대시보드 KPI(`kpi.Unpromoted`)에도 있는지.

사용자가 운영 DB에서 직접 확인할 쿼리(읽기 전용, 사용자가 실행):
```sql
SELECT s.sales_id, s.name, s.org_id, s.sales_owner, s.sales_owner_id, s.stage, s.close_reason
FROM sales_projects s
WHERE s.stage='closed' AND s.close_reason='contracted'
  AND NOT EXISTS (SELECT 1 FROM work_projects w WHERE TRIM(COALESCE(w.sales_project_id,''))=s.sales_id);

SELECT user_id, username, full_name, role FROM users WHERE username='hjlee';
```
(`s.org_id`가 비었거나 도서관 조직으로 들어가 있으면 데이터 문제 + 코드 문제 둘 다 있는 것. 사용자가 결과를 주면 Claude가 판정한다. 데이터 보정은 **코드로 하지 않는다**.)

## 2단계 — 권한 규칙 (서버 단일 판정 함수)
`handler`에 신규 함수 하나로 통일한다(UI·서버 가드 모두 이것만 사용).

```
canManageSalesProject(c, p *model.SalesProject) bool
  - isReadOnly(c) (c_level 등)            → false
  - vision_admin                          → true
  - p.OrgID == currentUserHomeOrg(c)      → canWriteSales(c) 결과를 따름 (조직 관리자·영업·권한자)
  - uid != "" && uid == p.SalesOwnerID    → canWriteSales(c) 결과를 따름 (타 조직 Owner 예외)
  - 그 외                                  → false
```
- "내 소속 조직" 은 로그인 사용자의 **홈 조직**(`org_id` 컨텍스트)이며, 조직 관리자의 `view_org_id` 전환값이나 시뮬레이션 값이 아님. `org_admin`은 홈 조직 사업에 한해 허용(`orgAdminInHome` 패턴 재사용).
- Owner 판정은 `SalesOwnerID` 우선. `SalesOwnerID`가 비어 있고 `SalesOwner`(이름)만 있는 레거시 건은 **이름 일치를 쓰지 않는다**(동명이인 위험). 대신 보고서에 해당 건수를 적는다.
- 적용 대상(서버 403 필수): `Edit`, `Update`, `ChangeStage`(수주 전환 포함), `ContractDialog`, `PromoteForm`, `PromoteCustomer`, `PromoteSave`, 파이프라인 단계 액션(`sales_pipeline_action.go`), `Drop`(기존 `canDropSales`와 통합), 견적/주문 등 사업에 딸린 쓰기 중 사업 상태를 바꾸는 것.
- **예외로 항상 허용**: 영업 활동 로그 등록/수정(`PermSalesActCreate`) — 타 조직 비Owner도 가능. 활동 등록 시 활동이 사업의 조직으로 저장되는 기존 규칙은 유지(변경 금지), 단 `person`·작성자는 본인.
- 거부 시 응답: 화면 전환은 기존 오류 팝업 규칙(95-12) 사용, 문구 `다른 조직의 사업입니다. 사업 Owner 또는 해당 조직만 변경할 수 있습니다.`

## 3단계 — 대시보드·목록 노출 제한
"내 업무 대시보드"(영업 대시보드, 프로젝트 대시보드, 홈 대시보드의 미전환·이관 확인·파이프라인·KPI·알림)는 **내 조직 사업 + 내가 Owner인 사업**만 포함한다.
- 신규 필터: `SalesListFilter`에 `OwnerUserID`(OR 조건용)를 두거나 서비스 레벨에서 `org_id = 내조직 OR sales_owner_id = uid`로 조회. 기존 `Owner`(이름 필터)와 의미 혼동 금지 — 별도 필드 `MyScope`로 구분.
- `ListContractUnion`/`listUnlinkedContractedSales`: 조직이 빈 값이면 전체로 폴백하는 현재 동작을 **대시보드 호출에서는 금지**(빈 조직 → 0건 또는 오류, 로그 남김). 전체 조회가 필요한 곳(vsadmin, C-Level, 계약 목록 화면 등)은 명시적으로 `OrgAll`을 넘기게 바꾼다.
- vision_admin / C-Level: 기존대로 전체(`OrgAll`) 보기 유지(읽기 전용은 전환 링크 숨김).
- **사업 목록(/sales)·상세 조회**는 95-2E대로 전 직원 조회 유지. 단 목록·상세·모달의 `수정/계약으로 전환/단계 변경` 버튼은 `canManageSalesProject` 결과로만 표시(템플릿에 `CanManage` 플래그를 행 단위로 전달; 파이프라인 카드는 행마다).
- 타 조직 사업 행에는 조직 뱃지(예: `SI사업팀`)를 표시하고, 쓰기 불가 사유 툴팁 `타 조직 사업 · Owner만 변경`.

## 4단계 — 템플릿 수정 위치 (전환/수정 링크는 모두 행 단위 `CanManage`)
`sales/dashboard.html:17`, `project/dashboard.html:127`, `sales/pipeline.html:89-91`, `sales/show.html:34,536`, `sales/modal.html:15`, `contracts/list.html:105` — `$.CanWriteSales` 단독 조건을 `.CanManage`(행별) 조건으로 교체. 새 행 구조체/뷰모델에 `CanManage bool` 필드를 추가해 핸들러에서 채운다.

## 테스트 (필수)
Go 핸들러/HTTP 테스트 (조직 A=도서관, B=SI, B에 사업 P_B(Owner=bUser), 도서관 영업 hjlee):
1. hjlee 영업 대시보드에 P_B가 **나타나지 않음**(미전환·파이프라인·이관 확인·KPI 모두).
2. P_B의 Owner를 hjlee로 바꾸면 대시보드에 나타나고 `계약으로 전환` 링크 표시, 수정·단계 변경·promote 성공.
3. hjlee가 P_B에 대해 `POST /sales/:id/stage`, `GET /sales/:id/promote`, `POST` promote 저장, `contract-dialog`, `Edit/Update`를 직접 호출 → **모두 403**(Owner 아닐 때).
4. hjlee가 P_B에 **영업 활동 등록** → 성공(허용 예외).
5. 도서관 조직의 P_A(Owner=타 사용자)는 hjlee가 기존대로 수정·전환 가능(홈 조직 규칙).
6. org_admin(도서관)이 P_B 수정·전환 시도 → 403(전환값 `view_org_id`로 우회 불가). vision_admin → 허용. C-Level → 조회만, 링크 숨김.
7. `sales_projects.org_id`가 빈 사업: 대시보드에서 전체 노출로 폴백되지 않음(0건), 사업 목록 조회는 기존 정책대로.
8. `ListContractUnion` 빈 조직 폴백 변경에 대한 회귀 테스트(계약 목록 화면의 vsadmin 전체 보기는 그대로).
9. 렌더 테스트: 타 조직·비Owner 행에 `계약으로 전환`/`수정` 버튼이 HTML에 없음, 조직 뱃지가 있음.
10. `go test ./...` 신규 실패 0(baseline `docs/리뷰/gotest-baseline-72A2.md` 이름 대조), `npm run build:css` 같은 커밋, `ui_lint` 0.

브라우저 확인(TEST 데이터는 확인 후 원복): hjlee 시점에서 영업 대시보드·프로젝트 대시보드·파이프라인·사업 상세에서 타 조직 사업의 전환/수정 버튼 없음, URL 직접 접근 시 오류 팝업, 영업 활동 등록 가능, Owner 지정 후 노출·전환 가능.

## 하지 말 것
- 데이터(조직·Owner) 보정을 코드로 일괄 수정하지 말 것(보정은 사용자가 운영에서 별도 수행).
- 95-2E 사업 조회 전 직원 공개 정책 변경 금지. 고객·AS·일일업무·점검 사이트의 조직 격리 규칙 변경 금지.
- 마이그레이션 없음 예상. 필요하면 별도 커밋 + `.down.sql`.
- 비밀번호·실이메일 로그/문서 금지, `server/data` 커밋 금지, `git add .` 금지.

## 보고 형식
1단계 원인(어느 경로·어떤 org_id), 커밋 해시, 버전/빌드 시각, 변경 파일, 신규 테스트 이름, go test 신규 실패 0 확인, Owner 이름만 있고 ID 없는 레거시 사업 건수, 남은 이슈.
