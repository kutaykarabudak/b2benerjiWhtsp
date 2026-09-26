# Firestore sunucusuz geçişi

Bu dal, canlı B2B Enerji WhatsApp panelini PostgreSQL ve Redis bağımlılığından
çıkarıp Firestore tabanlı düşük maliyetli çalışma zamanına taşır.

## Değişmez güvenlik kuralları

- Preview tamamlanmadan canlı Firebase Hosting, Meta callback veya `whatomate`
  Cloud Run trafiği değiştirilmez.
- Preview ayrı servis hesabı, ayrı Cloud Run servisi ve
  `whatomateNamespaces/whatomate-v1-preview` namespace'i kullanır.
- SQL yalnızca delta aktarımı ve doğrulama için okunur; kesimden önce silinmez.
- Aynı projedeki Academy Firestore kuralları ve indeksleri topluca deploy edilmez.
  Whatomate indeksleri yalnız eklemeli olarak oluşturulur.
- Tarayıcı Firestore'a doğrudan bağlanmaz. Admin SDK kullanan Cloud Run dışında
  `whatomateNamespaces` verisine istemci erişimi yoktur.

## Hedef mimari ve maliyet

- Firebase Hosting: Vue paneli ve değişmeyen webhook adresi.
- Cloud Run: API ve webhook, `min=0`, `max=1`, request CPU.
- Firestore Native: kalıcı uygulama verisi, webhook idempotency ve oturumlar.
- Cloud Storage: mevcut medya bucket'ı.
- Secret Manager: mevcut şifreleme, JWT ve medya anahtarları.
- Redis yerine tek instance ile uyumlu süreç içi sınırlı cache/rate limiter.
- Görünür tarayıcı sekmesinde 15 saniyelik cursor polling; gizli sekmede sorgu yok.

Provision edilmiş SQL/Redis yeni mimaride yoktur. Firestore sorguları sayfalıdır;
managed TTL ücretsiz kotaya dahil olmadığı için etkin değildir. Meta WhatsApp
konuşma ücretleri Google altyapı bütçesinden ayrıdır.

Resmi ücretsiz sınırlar:

- Firestore Standard: 1 GiB veri; günlük 50.000 okuma, 20.000 yazma ve 20.000 silme.
- Cloud Run: aylık ücretsiz istek, CPU ve bellek kotası içinde sıfıra yakın taban maliyet.
- Firebase Hosting: ücretsiz depolama ve aktarım kotası.
- 50 TRY bütçe alarmı korunur; alarm sert harcama limiti değildir.

## Firestore yerleşimi

```text
whatomateNamespaces/{namespace}
  organizations/{organizationId}
    contacts/{contactId}
      messages/{messageId}
      notes/{noteId}
    whatsAppAccounts/{accountId}
    templates/{templateId}
    tags/{tagHash}
    agentTransfers/{transferId}
    activeTransfers/{contactId}
    chatbotSessions/{sessionId}
    cannedResponses/{responseId}
    webhookReceipts/{sha256(externalMessageId)}
  users/{userId}
  userEmails/{sha256(normalizedEmail)}
  refreshTokens/{sha256(jti)}
```

## 26 Eylül 2026 doğrulama durumu

- Kaynak dal: `feat/firestore-serverless`, taban `origin/main@113b18b`.
- Canlı Firestore revizyonu: `whatomate-firestore-preview-00002-plb`.
- İmaj: `preview-20260926-2`, digest
  `sha256:549c3532349a7095c03991fe25210562bc06a24120a8258c922c2af2feb543ec`.
- İlk aktarım: 1 organizasyon, 1 kullanıcı, 3 hesap, 86 şablon,
  1.544 kişi, 21.316 mesaj, 727 transfer, 11 chatbot oturumu ve 1 hazır yanıt.
- Yedi Whatomate composite indeksi `READY`.
- Preview sağlık/hazır olma, giriş, kişiler, mesaj geçmişi, oturum verisi,
  transferler, şablonlar ve hazır yanıtlar smoke testinden geçti.
- Aktif transfer toplamı ve genel kuyruk sayısı `727` olarak doğrulandı.
- `go test ./...`, `go vet ./...`, production frontend build ve üretim bağımlılığı
  güvenlik taraması geçti (`npm audit --omit=dev`: 0 açık).
- Canlı Hosting sürümü `7c1167ab84acef83`, pin etiketi
  `fh-7c1167ab84acef83` ile Firestore servisine bağlıdır.
- SQL final delta iki kez koşullu olarak işlendi; canlı Hosting smoke testi geçti.
- `whatomate-db`, `whatomate-redis` ve emekli `whatomate` Cloud Run servisi
  kaldırıldı. SQL final yedeği 3 Ekim 2026'ya kadar korunur.

## Uygulanan kesim sırası

1. `2026-09-26T09:13:00Z` watermark'ından final delta migration çalıştırıldı.
2. SQL ve Firestore sayımları ve preview smoke testi tekrar doğrulandı.
3. Hosting rewrite'ı `whatomate-firestore-preview` servisine atomik olarak yayınlandı.
   Meta callback URL değişmez; yalnız arkasındaki servis değişir.
4. Kesim aralığındaki kayıtlar için aynı watermark ile ikinci koşullu delta çalıştırıldı.
5. Hosting URL'sinde giriş, inbox, health ve ready smoke testleri geçti; hata logu yok.
6. Yedi günlük final SQL yedeği oluşturuldu.
7. Cloud SQL, Redis ve artık çalışamayacak eski Cloud Run servisi kaldırıldı.

Resmi kaynaklar:

- https://docs.cloud.google.com/firestore/quotas
- https://cloud.google.com/run/pricing
- https://firebase.google.com/docs/hosting/usage-quotas-pricing
- https://cloud.google.com/secret-manager/pricing
