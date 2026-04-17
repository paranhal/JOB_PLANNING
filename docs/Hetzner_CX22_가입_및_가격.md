# Hetzner CX22 가입 방법 및 가격 정보

Hetzner(헤츠너)는 독일 본사의 클라우드·전용 서버 제공 업체입니다. **CX22**는 공유 vCPU 클라우드 서버 플랜 중 하나였습니다.

---

## 1. CX22 가격 및 사양 (기존 플랜 기준)

| 항목 | 내용 |
|------|------|
| **가격** | **€3.79/월** (VAT 제외 시) |
| **시간 단위** | €0.0050/시간 (최소 계약 없음, 사용한 만큼 과금 가능) |
| **CPU** | 2 vCPU (Intel Xeon Gold, 공유 vCPU) |
| **메모리** | 4 GB RAM |
| **스토리지** | 40 GB SSD |
| **트래픽** | 월 20 TB 포함, 초과 시 €0.001/GB |
| **IPv4** | 1개 포함 (IPv4 불필요 시 €0.50/월 할인 가능) |
| **가상화** | KVM |

- **용도:** 개발/테스트, 블로그, 소규모 DB, VPN, CMS 등 저부하 워크로드에 적합했습니다.

---

## 2. CX22 단종(Deprecated) 안내

- Hetzner는 **CX Gen2** 계열(CX22 포함)을 **Deprecated(단종)** 처리했습니다.
- **2026년 2월 13일** 기준으로 CX22는 **신규 주문 불가**이며, 기존에 사용 중인 서버는 당분간 정상 운영됩니다.
- 2025년 10월 경 서버 라인업 개편으로 **Regular Performance / Cost Optimized** 등 새 플랜으로 전환되었습니다.

**정리:** 지금 새로 가입해서 **CX22를 선택하는 것은 불가능**합니다. 비슷한 가격·사양을 쓰려면 아래 대체 플랜을 봐야 합니다.

---

## 3. CX22 대체 플랜 (비슷한 가격대)

| 플랜 | 가격(대략) | 사양 | 비고 |
|------|------------|------|------|
| **CAX11** | €3.79/월 | ARM, 2 vCPU, 4GB RAM, 40GB SSD | CX22와 동일 가격대 |
| **CPX11** | €4.35/월 | AMD, 2 vCPU, 2GB RAM, 40GB SSD | AMD 공유 vCPU |
| **Cost Optimized / Regular** | 사이트 기준 | 새 라인업 최소 사양 | 콘솔·가격 계산기에서 확인 |

- 실제 제공 여부·가격은 **Hetzner Cloud 요금 페이지·콘솔**에서 최신 정보를 확인하는 것이 좋습니다.  
- 공식: https://www.hetzner.com/cloud  
- 단종 플랜 공식 안내: https://docs.hetzner.com/cloud/servers/deprecated-plans/

---

## 4. Hetzner 가입 방법 (공통)

CX22는 더 이상 선택할 수 없지만, **Hetzner Cloud** 자체 가입 절차는 다음과 같습니다.

### 4-1. 가입 절차

| 순서 | 내용 |
|------|------|
| 1 | **가입 페이지** 접속: https://accounts.hetzner.com/login 또는 https://hetzner.cloud → **"REGISTER NOW"** 클릭 |
| 2 | **이메일·비밀번호** 입력 (비밀번호 12자 이상 권장) |
| 3 | **이메일 인증** — 발송된 메일의 링크 클릭 |
| 4 | **프로필** — 개인/조직 선택, 이름 등 입력 |
| 5 | **청구 정보** — 주소, 전화번호, 통화(EUR 또는 USD), **결제 수단** 등록 (신용카드 등) |

### 4-2. 필요 정보

- 유효한 **이메일 주소** (가능하면 무료 이메일은 피하는 것이 좋다고 안내하는 경우 있음)
- **청구용 주소**
- **결제 수단** (카드 등)
- 개인/사업자 **연락처**

### 4-3. 주의사항 (Hetzner 권장)

- 가입 시 **VPN** 사용을 피할 것 (계정 검증에 불리할 수 있음)
- **2단계 인증(2FA)** 설정 권장
- 서버 생성 전 **SSH 키** 등록 권장

### 4-4. 가입 후 서버 생성

1. https://console.hetzner.com/ 로그인  
2. **Cloud** → **Add Server** (또는 **Create cloud instance**)  
3. **리전** 선택: 독일(Falkenstein), 핀란드(Helsinki), 미국(Ashburn/Hillsboro), 싱가포르 등  
4. **이미지**: Ubuntu, Debian 등 선택  
5. **서버 타입**: CX22는 목록에 없으므로 **CAX11, CPX11** 또는 **Cost Optimized / Regular** 중 최소 사양 선택  
6. SSH 키·이름 입력 후 생성  

---

## 5. 요약

| 항목 | 내용 |
|------|------|
| **CX22 가격** | €3.79/월 (2 vCPU, 4GB RAM, 40GB SSD) |
| **CX22 상태** | 단종됨 — **신규 가입·신규 서버로는 선택 불가** |
| **대체** | CAX11(€3.79), CPX11(€4.35) 또는 새 Cost Optimized/Regular 플랜 확인 |
| **가입** | https://accounts.hetzner.com 또는 https://hetzner.cloud → Register → 이메일 인증 → 청구·결제 정보 입력 |
| **서버 생성** | https://console.hetzner.com → Cloud → Add Server |

실제 이용 가능한 플랜과 최신 가격은 반드시 **Hetzner 공식 사이트**에서 확인하세요.
