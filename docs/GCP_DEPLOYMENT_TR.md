# Google Cloud Run + Firebase Hosting kurulumu

Bu dağıtımda dashboard, API ve WebSocket aynı `PROJECT_ID.web.app` origin'i üzerinden çalışır. Firebase Hosting istekleri `europe-west1` bölgesindeki `whatomate` Cloud Run servisine yönlendirir.

## Mimari

- Firebase Hosting: herkese verilecek `web.app` adresi
- Cloud Run: Go API, gömülü Vue dashboard ve tek kampanya worker'ı
- Cloud SQL for PostgreSQL: kalıcı uygulama verisi
- Redis/Memorystore: kuyruk, rate limit, token rotation ve gerçek zamanlı olaylar
- Secret Manager: uygulama şifreleme anahtarı, JWT, DB/Admin parolaları
- Meta Cloud API: `/api/webhook` callback'i

Firebase Hosting yalnızca `__session` isimli cookie'yi Cloud Run'a ilettiği için proje, Whatomate'ın iki imzalı JWT'sini bu HttpOnly cookie içinde taşıyacak şekilde uyarlanmıştır. Standart self-host kurulumundaki cookie davranışı `cookie.firebase_hosting=false` iken değişmez.

## 1. Ön koşullar

VS Code terminalinde şu araçlarla giriş yapın:

```bash
gcloud auth login
gcloud auth application-default login
firebase login
```

Google Cloud projesinde faturalandırma açık olmalıdır.

## 2. PostgreSQL ve Redis

Cloud SQL üzerinde PostgreSQL instance, `whatomate` veritabanı ve `whatomate` kullanıcısı oluşturun. Instance connection name değerini not edin:

```text
PROJECT_ID:europe-west1:whatomate-db
```

Redis için aynı bölgede Memorystore kullanabilirsiniz. Private IP kullanıyorsanız Cloud Run Direct VPC için network ve subnet adlarını deployment env dosyasına yazın. Alternatif olarak TLS destekli erişilebilir bir Redis servisi kullanılabilir.

## 3. Secret'ları oluşturun

Anahtarlar komut satırı argümanına, dosyaya veya GitHub'a yazılmaz. Script değerleri gizli terminal girişiyle doğrudan Secret Manager'a yollar:

```bash
PROJECT_ID=your-project scripts/prepare-secrets.sh
```

`whatomate-db-password`, Cloud SQL kullanıcısına verdiğiniz parola ile aynı olmalıdır. İlk admin parolasını kaybetmeyin.

## 4. Cloud Run dağıtımı

```bash
cp deploy/cloudrun.env.example deploy/cloudrun.env
```

`deploy/cloudrun.env` dosyasını doldurun. Bu dosya `.gitignore` kapsamındadır. Ardından:

```bash
scripts/deploy-gcp.sh
```

Cloud Run minimum 1 instance ve CPU throttling kapalı olarak kurulur; bunun nedeni kampanya worker'ının HTTP isteği yokken de kuyruk tüketmesidir.

## 5. Firebase Hosting

`firebase.json`, servis adı `whatomate` ve bölge `europe-west1` kabul eder:

```bash
scripts/deploy-firebase.sh your-project-id
```

Dashboard adresi `https://your-project-id.web.app` olur. Firebase'in Cloud Run proxy'si WebSocket isteklerini 60 saniyede sonlandırabilir; mevcut istemci otomatik bağlanır ve kaçırılan veriyi yeniler.

## 6. Meta hesabını bağlayın

Dashboard'a admin hesabıyla giriş yapıp Settings > Accounts bölümünden aşağıdaki değerleri ekleyin:

- Meta App ID
- Phone Number ID
- WhatsApp Business Account ID
- System User Access Token
- Meta App Secret

Access token ve App Secret, Secret Manager'daki `whatomate-encryption-key` kullanılarak PostgreSQL içinde AES-256-GCM ile şifrelenir ve API cevaplarında geri döndürülmez.

Meta webhook ayarları:

```text
Callback URL: https://PROJECT_ID.web.app/api/webhook
Verify token: Dashboard'daki WhatsApp account kaydında gösterilen değer
```

Webhook'ta en az `messages` alanına abone olun. Üretime çıkmadan önce test kişileriyle onaylı bir template kampanyası çalıştırın.

## Güvenlik notları

- `config.toml`, `deploy/cloudrun.env` ve `.env` dosyalarını commit etmeyin.
- Meta tokenlarını frontend `VITE_*` değişkenlerine koymayın.
- Secret değerlerini Cloud Run normal environment variable olarak değil Secret Manager referansı olarak bağlayın.
- `whatomate-encryption-key` kaybolursa veritabanındaki Meta tokenları çözülemez; güvenli yedek ve kontrollü rotation planı gerekir.
- Webhook signature doğrulaması için her WhatsApp account kaydına Meta App Secret ekleyin.
- Yalnızca açık rıza/opt-in alınmış kişilere onaylı template gönderin ve opt-out taleplerini uygulayın.
