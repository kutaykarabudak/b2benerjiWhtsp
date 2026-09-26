# Google Cloud Run + Firestore dağıtımı

Canlı B2B Enerji WhatsApp paneli sunucusuz ve düşük maliyetli çalışır:

- Firebase Hosting, paneli ve API/webhook yönlendirmesini sunar.
- `whatomate-firestore-preview` Cloud Run servisi API ile gömülü paneli çalıştırır.
- Firestore Native kalıcı uygulama verisini tutar.
- Cloud Storage sohbet ve kampanya medyasını tutar.
- Secret Manager yalnızca çalışma zamanında gereken anahtarları tutar.

Cloud SQL, PostgreSQL, Redis, Memorystore, VPC connector ve sürekli açık Cloud Run
instance'ı bu mimarinin parçası değildir.

## Gerekli secret'lar

Çalışan revizyon yalnızca aşağıdaki secret'ları kullanır:

- `whatomate-encryption-key`
- `whatomate-jwt-secret`
- `whatomate-media-s3-key`
- `whatomate-media-s3-secret`

Uygulama yönetici parolasını Secret Manager'dan okumaz. Kullanıcı ve parola özeti
Firestore'a aktarılmıştır. SQL ve Redis parolaları artık oluşturulmamalıdır.

Temel uygulama anahtarlarını güvenli terminal girdisiyle hazırlamak için:

```bash
PROJECT_ID=b2benerji-whatsapp-2026 scripts/prepare-secrets.sh
```

Medya HMAC anahtarları mevcut bucket/runtime hesabıyla birlikte korunmalıdır.

## İmaj ve Cloud Run dağıtımı

PowerShell'de önce imajı oluşturun:

```powershell
.\scripts\build-firestore-preview.ps1 `
  -Tag "release-$(Get-Date -Format yyyyMMdd-HHmm)" `
  -Execute `
  -ConfirmProject b2benerji-whatsapp-2026
```

Oluşan imaj URI'sini gözden geçirip servise dağıtın:

```powershell
.\scripts\deploy-firestore-preview.ps1 `
  -Image "europe-west1-docker.pkg.dev/b2benerji-whatsapp-2026/whatomate/IMAGE:TAG" `
  -Execute `
  -ConfirmProject b2benerji-whatsapp-2026
```

Dağıtım scripti maliyet korumalarını uygular: `min-instances=0`, `max-instances=1`,
request CPU, 512 MiB bellek, Cloud SQL bağlantısı yok ve VPC connector yok.

## Firebase Hosting

Cloud Run revizyonu doğrulandıktan sonra Hosting dağıtımı yapılır:

```bash
scripts/deploy-firebase.sh b2benerji-whatsapp-2026
```

Canlı adres `https://b2benerji-whatsapp-2026.web.app` olarak kalır. Meta callback
adresi de değişmez: `https://b2benerji-whatsapp-2026.web.app/api/webhook`.

## Güvenlik ve maliyet kontrolleri

- Secret değerlerini komut satırına, env dosyasına veya Git'e yazmayın.
- `whatomate-encryption-key` ve medya HMAC anahtarlarını doğrulamadan silmeyin.
- Yeni revizyonu smoke test etmeden Hosting trafiğini değiştirmeyin.
- Cloud Run minimum instance değerini sıfırda, maksimum instance değerini birde tutun.
- Firestore sorgularında cursor ve limit kullanın; toplu sınırsız tarama eklemeyin.
- 50 TRY bütçe uyarısı harcamayı otomatik durdurmaz; faturalandırma ekranı ayrıca izlenmelidir.

Geçiş ve doğrulama ayrıntıları için `docs/FIRESTORE_MIGRATION_TR.md` dosyasına bakın.
