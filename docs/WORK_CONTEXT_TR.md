# B2B Enerji WhatsApp Paneli - Çalışma Bağlamı ve Çalışma Ağacı

Bu dosya başka bir workspace/oturumda projeye kaldığımız yerden devam etmek için hazırlanmıştır.

Son güncelleme: 2026-06-30

## 1. Proje özeti

Amaç: Meta resmi WhatsApp Cloud API ile çalışan, Cloud Run üzerinde backend ve Firebase Hosting `web.app` linki üzerinden kullanılan B2B Enerji WhatsApp dashboardu.

Canlı adresler:

- Panel: `https://b2benerji-whatsapp-2026.web.app`
- Cloud Run backend: `https://whatomate-702123018184.europe-west1.run.app`
- Health check: `https://b2benerji-whatsapp-2026.web.app/health`
- Gizlilik politikası: `https://b2benerji-whatsapp-2026.web.app/privacy`

Repo:

- GitHub origin: `https://github.com/kutaykarabudak/b2benerjiWhtsp`
- Upstream kaynak: `https://github.com/shridarpatil/whatomate.git`
- Ana branch: `main`

## 2. Altyapı / GCP bilgileri

GCP:

- Project ID: `b2benerji-whatsapp-2026`
- Project number: `702123018184`
- Region: `europe-west1`
- Cloud Run service: `whatomate`
- Firebase Hosting site: `b2benerji-whatsapp-2026`
- Cloud SQL: `whatomate-db`
- PostgreSQL sürümü: 15
- DB adı / kullanıcısı: `whatomate`
- Redis: `whatomate-redis`
- Redis host: `10.164.75.163`
- VPC: `whatomate-vpc`
- Subnet: `whatomate-europe-west1`
- Runtime service account: `whatomate-runtime@b2benerji-whatsapp-2026.iam.gserviceaccount.com`
- Medya bucket (GCS, S3-uyumlu XML interop ile): `b2benerji-whatsapp-2026-media` (europe-west1). Sohbet/kampanya medyası buraya yazılır — yerel disk Cloud Run'da redeploy/restart/instance değişiminde sıfırlanır, bu yüzden `storage.type=s3` zorunlu (2026-08-10'da eklendi, bkz. `internal/handlers/media.go`).

Secret Manager içindeki secret adları:

- `whatomate-admin-password`
- `whatomate-db-password`
- `whatomate-encryption-key`
- `whatomate-jwt-secret`
- `whatomate-redis-password`
- `whatomate-media-s3-key` / `whatomate-media-s3-secret` — medya bucket'ının HMAC anahtarı (runtime servis hesabına ait)

Admin:

- Admin e-posta: `b2benerji@gmail.com`
- Admin şifresi repo içinde yoktur. Gerekirse şu komutla GCP Secret Manager’dan okunur:

```bash
gcloud secrets versions access latest \
  --secret=whatomate-admin-password \
  --project=b2benerji-whatsapp-2026
```

Önemli güvenlik notu:

- `deploy/cloudrun.env` gerçek ortam değerleri içerir ve `.gitignore` içindedir. Commit edilmemelidir.
- `deploy/cloudrun.env.example` sadece örnek placeholder değerler içerir ve commit edilebilir.

## 3. Bugüne kadar yapılan işler

### Kurulum ve deploy

- Whatomate kaynak kodu bu repo içine alındı.
- Cloud Run için deploy scriptleri eklendi.
- Firebase Hosting ile `web.app` üzerinden panel yayına alındı.
- Cloud SQL, Redis, VPC, Secret Manager ve runtime service account yapısı hazırlandı.
- Firebase Hosting rewrite ile frontend ve backend aynı domain altında çalışacak şekilde ayarlandı.
- Health endpoint canlı olarak çalışıyor.

### Güvenlik / üretim sertleştirme

- Production ortamda zayıf/default JWT secret, encryption key ve admin password ile başlamayı engelleyen kontroller eklendi.
- Webhook imza doğrulaması güçlendirildi.
- Production webhook isteklerinde eksik/geçersiz imza reddedilecek şekilde düzenlendi.
- Firebase Hosting uyumu için session cookie yapısı `__session` altında toparlandı.
- Firebase arkasında CSRF origin kontrolü uyumlu hale getirildi.
- Public sign-up kapatıldı:
  - `/register` route kaldırıldı.
  - Login ekranındaki sign-up linki kaldırıldı.
  - Kullanıcı davet/kayıt linki arayüzden kaldırıldı.
  - Backend `/api/auth/register` endpoint’i `403` dönecek şekilde kapatıldı.

### Marka ve arayüz

- Panel markası B2B Enerji olarak değiştirildi.
- Login footer metni B2B Enerji yetkili kullanıcı paneli yapıldı.
- Varsayılan dil Türkçe yapıldı.
- Türkçe çeviri dosyası eklendi: `frontend/src/i18n/locales/tr.json`
- Türkçe karakter bozulmaları için görülen metinler düzeltildi.
- Ana menü sadeleştirildi:
  - Ana odak: Sohbet, Kampanyalar, Sohbet robotu.
  - Diğer ekranlar: Diğer İşler altında toplandı.
- Router ilk erişilebilir ekran sırası ana odakla uyumlu hale getirildi.

### Gizlilik politikası

- `/privacy` endpoint’i eklendi.
- Meta App yayınlama için kullanılabilecek gizlilik politikası canlı:
  - `https://b2benerji-whatsapp-2026.web.app/privacy`

### Meta WhatsApp test bağlantısı

Meta test kurulumu:

- Meta App ID: `2739072319715670`
- Business ID / portfolio URL parametresi: `23968095896159710`
- Test WhatsApp numarası: `+1 (555) 674-2947`
- Test Phone Number ID: `1238753685979907`
- Test WABA ID: `994073263510844`
- Panelde oluşturulan hesap adı: `DENEME`

Durum:

- Test token panelde kaydedildi.
- Webhook verify başarılı oldu.
- Uygulama webhook aboneliği başarılı oldu.
- Test mesajları kullanıcı numarasına geldi.
- Sonrasında kullanıcı “geldi mesaj hallettik” diyerek webhook/mesaj alma akışının çalıştığını doğruladı.

Güvenlik notu:

- Geçici test access token sohbet içinde paylaşılmıştır. Bu token repo içinde yazılmadı ve commit edilmedi. Yine de artık açık kabul edilmeli; devam etmeden önce Meta’dan yeni test token üretmek veya production için kalıcı system-user token kullanmak gerekir.

## 4. Panel kullanımı için mevcut akış

### Hesap ayarı

Konum:

- Diğer İşler > Hesaplar > `DENEME`

Gerekli alanlar:

- Meta App ID
- Phone Number ID
- WABA ID
- API version
- Access Token
- App Secret

Webhook alanında panel şunları gösterir:

- Callback URL: `https://b2benerji-whatsapp-2026.web.app/api/webhook`
- Verify token: panelde üretilen doğrulama jetonu

Meta tarafında webhook kurulumu için Callback URL ve Verify token birebir girilir.

### Sohbet

Konum:

- Ana Odak > Sohbet

Amaç:

- Gelen/giden WhatsApp konuşmalarını görmek.
- Testte gelen mesajlar bu ekranda takip edilir.

### Kişiler

Konum:

- Diğer İşler > Kişiler

Amaç:

- Mesaj gönderilecek kişileri eklemek.
- Toplu mesaj / kampanya için kişi listesi hazırlamak.

### Şablonlar

Konum:

- Diğer İşler > Şablonlar

Önemli kullanım notu:

- Meta’dan senkronize et butonu, hesap seçimi `Tüm hesaplar` iken API çağrısı yapmaz.
- Önce hesap filtresinden `DENEME` hesabı seçilmeli.
- Sonra Meta’dan senkronize et butonuna basılmalı.

Teknik not:

- Backend route mevcut: `POST /api/templates/sync`
- Önceki kontrolde butona basıldığında server loglarında `/api/templates/sync` çağrısı görünmedi. En olası neden hesap seçili olmamasıydı.

### Kampanyalar / toplu mesaj

Konum:

- Ana Odak > Kampanyalar

Beklenen akış:

1. Diğer İşler > Kişiler bölümünde kişileri ekle.
2. Diğer İşler > Şablonlar bölümünde Meta onaylı template’leri senkronize et.
3. Ana Odak > Kampanyalar bölümünde yeni kampanya oluştur.
4. Hesap olarak `DENEME` seç.
5. Hedef kişi/listeleri seç.
6. Meta onaylı template seç.
7. Gönderimi test et.

WhatsApp Cloud API kuralı:

- Kullanıcı işletmeye son 24 saat içinde mesaj attıysa serbest metin cevap gönderilebilir.
- İşletmenin kullanıcıya ilk mesajı veya 24 saat dışındaki mesajı için Meta onaylı template gerekir.

### Sohbet robotu / otomatik cevaplar

Konum:

- Ana Odak > Sohbet robotu
- İlgili alt ayarlar Diğer İşler altında Akışlar, Hazır Yanıtlar, Etiketler, Takımlar vb. bölümlerde bulunabilir.

Yapılacak ürün akışı:

- İlk mesaj karşılama.
- Anahtar kelimeye göre otomatik cevap.
- Temsilciye aktarma.
- Mesai dışı mesajı.
- Etiketleme ve kampanya segmentleri.

## 5. Çalışma ağacı / sıradaki işler

### A. Hemen yapılacak ürün işleri

1. Kampanya ile ilk toplu mesaj testini tamamla.
   - Kişiler eklendi.
   - Şablon seçimi ve hesap seçimi kontrol edilecek.
   - Test numarasına kampanya gönderimi denenmeli.

2. Template senkronizasyonunu netleştir.
   - Hesap filtresi `DENEME` olacak.
   - Senkronize et butonuna basıldığında Network/log’da `POST /api/templates/sync` görülmeli.
   - Görülmüyorsa frontend event/route bug’ı düzeltilmeli.

3. Chatbot kullanımını ürünleştir.
   - “İlk mesaj / hoş geldiniz” kuralı.
   - Anahtar kelime cevapları.
   - Mesai dışı cevap.
   - Temsilciye aktarma.
   - Test mesajlarıyla doğrulama.

4. Panelde ana odak deneyimini iyileştir.
   - Kampanya oluşturma akışını daha görünür hale getir.
   - Şablon seçimi yoksa kullanıcıyı önce şablon senkronizasyonuna yönlendir.
   - Test token / production token uyarısını hesap ekranında daha anlaşılır göster.

### B. Meta production geçiş işleri

1. Gerçek WhatsApp Business numarasını production setup altında ekle.
2. Business verification tamamlanacaksa Meta Business dokümanları hazırlanmalı.
3. Geçici test token yerine kalıcı system-user access token üret.
4. Token permissions:
   - `whatsapp_business_messaging`
   - `whatsapp_business_management`
5. App Secret panelde girilmeli.
6. Webhook subscriptions production WABA için tekrar doğrulanmalı.
7. B2B Enerji canlıya alınırken test `DENEME` değerleri gerçek Phone Number ID / WABA ID / token ile değiştirilmeli.

### C. Deploy/build hız problemi

Durum:

- Deploy şu anda çalışıyor ama hızlı değil.
- `cloudbuild.yaml` içinde Docker pull + BuildKit inline cache denendi.
- Buna rağmen gözlenen build süreleri yaklaşık 5 dakika civarında kaldı.
- `npm ci`, `apt-get install`, `go build` ve bazı sistem paketleri tekrar çalışıyor gibi görünüyor.
- Son gözlemlerden biri: build yaklaşık 5m13s; önceki build yaklaşık 5m55s.

Kök sorun:

- Cloud Build worker’ı ephemeral olduğu için lokal cache yok.
- Mevcut Dockerfile sistem paketleri, npm bağımlılıkları ve Go build aşamalarını aynı genel build akışında tekrar tetikliyor.
- Inline cache tek başına bu projede yeterli hız kazandırmadı.

Önerilen çözüm:

1. Stable base image oluştur:
   - ffmpeg
   - espeak
   - piper
   - Go build dependency ön hazırlıkları
   - Node/npm temel bağımlılık katmanları
2. Bu base image’i Artifact Registry’ye push et.
3. Ana uygulama Dockerfile’ını bu base image üstünden başlat.
4. Frontend dependency layer ve Go module layer’larını daha keskin ayır.
5. Cloud Build’te cache-from/cache-to yerine mümkünse Artifact Registry remote cache veya ayrı base image stratejisi kullan.
6. Deploy scriptte sadece değişen app layer’larını build edecek düzen kur.

Hedef:

- Her küçük arayüz/backend değişikliğinde sistem paketleri tekrar kurulmasın.
- Normal deploy birkaç dakikanın altına düşsün.

### D. Güvenlik / operasyon

1. Sohbette paylaşılan geçici test token rotate edilmeli.
2. Production için token asla chat, README veya repo dosyalarına yazılmamalı.
3. `deploy/cloudrun.env` lokal/secret amaçlıdır; commit edilmemeli.
4. Admin şifresi sadece Secret Manager üzerinden yönetilmeli.
5. Eğer repo public kalacaksa:
   - `.gitignore` tekrar kontrol edilmeli.
   - Secret taraması yapılmalı.
   - GitHub secret scanning açık tutulmalı.

## 6. Yeni workspace için başlangıç kontrol listesi

Yeni workspace açıldığında:

```bash
git clone https://github.com/kutaykarabudak/b2benerjiWhtsp
cd b2benerjiWhtsp
git status
```

Deploy yapılacaksa:

```bash
gcloud auth login
gcloud config set project b2benerji-whatsapp-2026
firebase login
```

Yerel secret/env dosyası gerekiyorsa:

```bash
cp deploy/cloudrun.env.example deploy/cloudrun.env
```

Not: Gerçek değerler Secret Manager’dan veya güvenli kaynaktan doldurulmalı; repo içine yazılmamalı.

## 7. Son doğrulamalar

Bu bağlam dosyası oluşturulmadan önce bilinen başarılı kontroller:

- Frontend build daha önce başarılı oldu.
- Go testleri çalıştırıldı.
- `/privacy` endpoint’i canlıda HTTP 200 döndü.
- `/health` endpoint’i canlıda çalışıyor.
- Meta webhook doğrulama ve mesaj alma akışı testte başarıyla görüldü.

## 8. Commit öncesi dikkat

Commit’e girmemesi gerekenler:

- `deploy/cloudrun.env`
- Gerçek WhatsApp access token
- Admin şifresi
- DB/Redis/JWT/encryption secret değerleri

Commit’e girebilecekler:

- Kod değişiklikleri
- Deploy scriptleri
- Example env dosyası
- GCP deploy dokümanı
- Bu çalışma bağlamı dosyası
