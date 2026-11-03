# 모바일 화면 점검 (95-69 ①)

폭 390·360에서 A등급 화면을 캡처하고, 가로 스크롤·잘린 표·44px 미만 터치·고정 영역 겹침을 센다. **화면 코드는 이 스크립트가 고치지 않는다.**

```sh
export MOBILE_AUDIT_BASE=http://127.0.0.1:8080
export MOBILE_AUDIT_USER=...
export MOBILE_AUDIT_PASS=...
cd scripts/mobile_audit
go test
go run . -grade A
```

보고서는 `docs/리뷰/95-69단계_모바일_화면_점검.md`, 캡처는 `docs/리뷰/screenshots/95-69/`.
