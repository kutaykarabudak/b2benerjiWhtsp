<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  listCatalogs,
  createCatalog,
  syncCatalogs,
  activateCatalog,
  deleteCatalog,
  listProducts,
  createProduct,
  updateProduct,
  deleteProduct,
  uploadCatalogImage,
  type Catalog,
  type Product,
  type ProductInput
} from '@/services/catalog'
import {
  listAccounts,
  updateAccountCatalogBusinessID,
  type WhatsAppAccount as Account
} from '@/services/admin'

const accounts = ref<Account[]>([])
const account = ref('')
const catalogs = ref<Catalog[]>([])
const selectedCatalogId = ref('')
const products = ref<Product[]>([])
const loading = ref(true)
const loadingProducts = ref(false)
const syncing = ref(false)
const activating = ref(false)
const notice = ref<{ type: 'success' | 'error'; text: string } | null>(null)
const search = ref('')
const catalogBusinessId = ref('')
const savingCatalogSettings = ref(false)

const showCatalogModal = ref(false)
const newCatalogName = ref('')
const creatingCatalog = ref(false)

const showProductModal = ref(false)
const editingProduct = ref<Product | null>(null)
const savingProduct = ref(false)
const productError = ref('')
const pForm = ref(blankProduct())
const productImageFile = ref<File | null>(null)
const productImagePreview = ref('')
const uploadingProductImage = ref(false)

function blankProduct(): ProductInput {
  return { name: '', description: '', price: null, currency: 'TRY', image_url: '', url: '', retailer_id: '', availability: 'in stock', condition: 'new' }
}

const selectedCatalog = computed(() => catalogs.value.find((item) => item.id === selectedCatalogId.value) || null)
const currentAccount = computed(() => accounts.value.find((item) => item.name === account.value) || null)
const catalogConfigured = computed(() => !!currentAccount.value?.catalog_business_id)
const totalProducts = computed(() => catalogs.value.reduce((total, item) => total + item.product_count, 0))
const filteredProducts = computed(() => {
  const query = search.value.trim().toLocaleLowerCase('tr-TR')
  if (!query) return products.value
  return products.value.filter((item) =>
    [item.name, item.description, item.retailer_id].some((value) => value?.toLocaleLowerCase('tr-TR').includes(query))
  )
})

async function initialize() {
  loading.value = true
  try {
    accounts.value = await listAccounts()
    account.value = accounts.value[0]?.name || ''
    catalogBusinessId.value = accounts.value[0]?.catalog_business_id || ''
    await loadCatalogs()
  } catch (error: any) {
    showNotice('error', apiError(error, 'Katalog bilgileri yüklenemedi.'))
  } finally {
    loading.value = false
  }
}

async function loadCatalogs() {
  catalogs.value = await listCatalogs(account.value)
  const stillExists = catalogs.value.some((item) => item.id === selectedCatalogId.value)
  if (!stillExists) selectedCatalogId.value = catalogs.value[0]?.id || ''
  await loadProducts()
}

async function changeAccount() {
  selectedCatalogId.value = ''
  products.value = []
  loading.value = true
  catalogBusinessId.value = currentAccount.value?.catalog_business_id || ''
  try {
    await loadCatalogs()
  } finally {
    loading.value = false
  }
}

async function saveCatalogSettings() {
  const selected = currentAccount.value
  const value = catalogBusinessId.value.trim()
  if (!selected || !value) {
    showNotice('error', 'Bu WhatsApp hesabının sahibi olan Meta Business Portfolio ID’yi girin.')
    return
  }
  savingCatalogSettings.value = true
  try {
    await updateAccountCatalogBusinessID(selected.id, value)
    selected.catalog_business_id = value
    showNotice('success', `${selected.name} için katalog sahibi kaydedildi.`)
  } catch (error: any) {
    showNotice('error', apiError(error, 'Katalog ayarı kaydedilemedi.'))
  } finally {
    savingCatalogSettings.value = false
  }
}

async function selectCatalog(catalog: Catalog) {
  if (selectedCatalogId.value === catalog.id) return
  selectedCatalogId.value = catalog.id
  search.value = ''
  await loadProducts()
}

async function loadProducts() {
  if (!selectedCatalogId.value) {
    products.value = []
    return
  }
  loadingProducts.value = true
  try {
    products.value = await listProducts(selectedCatalogId.value)
  } catch (error: any) {
    showNotice('error', apiError(error, 'Ürünler yüklenemedi.'))
  } finally {
    loadingProducts.value = false
  }
}

async function addCatalog() {
  if (!newCatalogName.value.trim() || !account.value) return
  if (!catalogConfigured.value) {
    showCatalogModal.value = false
    showNotice('error', 'Önce bu hesaba ait Katalog Business Portfolio ID’yi kaydedin.')
    return
  }
  creatingCatalog.value = true
  try {
    await createCatalog(account.value, newCatalogName.value.trim())
    newCatalogName.value = ''
    showCatalogModal.value = false
    await loadCatalogs()
    selectedCatalogId.value = catalogs.value[catalogs.value.length - 1]?.id || selectedCatalogId.value
    await loadProducts()
    showNotice('success', 'Katalog Meta hesabında oluşturuldu.')
  } catch (error: any) {
    showNotice('error', apiError(error, 'Katalog oluşturulamadı.'))
  } finally {
    creatingCatalog.value = false
  }
}

async function doSync() {
  if (!account.value || syncing.value) return
  syncing.value = true
  try {
    const result = await syncCatalogs(account.value)
    await loadCatalogs()
    showNotice('success', `${result.synced} katalog ve ${result.products_synced} ürün Meta ile eşitlendi.`)
  } catch (error: any) {
    showNotice('error', apiError(error, 'Meta senkronizasyonu tamamlanamadı.'))
  } finally {
    syncing.value = false
  }
}

async function activateSelectedCatalog() {
  if (!selectedCatalog.value || activating.value) return
  activating.value = true
  try {
    await activateCatalog(selectedCatalog.value.id)
    showNotice('success', 'Katalog WhatsApp profiline bağlandı; katalog görünürlüğü ve sepet etkinleştirildi.')
  } catch (error: any) {
    showNotice('error', apiError(error, 'Katalog WhatsApp profilinde etkinleştirilemedi.'))
  } finally {
    activating.value = false
  }
}

async function removeCatalog(catalog: Catalog) {
  if (!confirm(`“${catalog.name}” ve içindeki ürünler silinsin mi?`)) return
  try {
    await deleteCatalog(catalog.id)
    await loadCatalogs()
    showNotice('success', 'Katalog silindi.')
  } catch (error: any) {
    showNotice('error', apiError(error, 'Katalog silinemedi.'))
  }
}

function openNewProduct() {
  clearProductImageSelection()
  editingProduct.value = null
  pForm.value = blankProduct()
  productError.value = ''
  showProductModal.value = true
}

function openEditProduct(product: Product) {
  clearProductImageSelection()
  editingProduct.value = product
  pForm.value = {
    name: product.name,
    description: product.description,
    price: product.price > 0 ? product.price / 100 : null,
    currency: product.currency,
    image_url: product.image_url,
    url: product.url,
    retailer_id: product.retailer_id,
    availability: product.availability || 'in stock',
    condition: product.condition || 'new'
  }
  productError.value = ''
  showProductModal.value = true
}

function clearProductImageSelection() {
  if (productImagePreview.value) URL.revokeObjectURL(productImagePreview.value)
  productImageFile.value = null
  productImagePreview.value = ''
}

function selectProductImage(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0] || null
  input.value = ''
  if (!file) return
  if (!['image/jpeg', 'image/png', 'image/webp'].includes(file.type)) {
    productError.value = 'Yalnızca JPG, PNG veya WEBP görsel yükleyin.'
    return
  }
  if (file.size > 8 * 1024 * 1024) {
    productError.value = 'Ürün görseli en fazla 8 MB olabilir.'
    return
  }
  clearProductImageSelection()
  productImageFile.value = file
  productImagePreview.value = URL.createObjectURL(file)
  productError.value = ''
}

async function saveProduct() {
  productError.value = ''
  if (!pForm.value.name.trim()) {
    productError.value = 'Ürün adı zorunlu.'
    return
  }
  if ((pForm.value.price || 0) < 0) {
    productError.value = 'Fiyat negatif olamaz.'
    return
  }
  if (!pForm.value.retailer_id.trim()) {
    productError.value = 'WhatsApp vitrini için benzersiz bir stok kodu / SKU girin.'
    return
  }
  if (!productImageFile.value && !pForm.value.image_url.trim()) {
    productError.value = 'WhatsApp vitrini için bir ürün görseli yükleyin.'
    return
  }
  if (!selectedCatalog.value) return

  savingProduct.value = true
  try {
    if (productImageFile.value) {
      uploadingProductImage.value = true
      pForm.value.image_url = await uploadCatalogImage(productImageFile.value)
      if (!pForm.value.image_url) throw new Error('Yüklenen görsel adresi alınamadı.')
    }
    if (editingProduct.value) await updateProduct(editingProduct.value.id, pForm.value)
    else await createProduct(selectedCatalog.value.id, pForm.value)
    clearProductImageSelection()
    showProductModal.value = false
    await Promise.all([loadProducts(), refreshCatalogCounts()])
    showNotice('success', editingProduct.value ? 'Ürün güncellendi.' : 'Ürün kataloğa eklendi.')
  } catch (error: any) {
    productError.value = apiError(error, 'Ürün kaydedilemedi.')
  } finally {
    uploadingProductImage.value = false
    savingProduct.value = false
  }
}

async function removeProduct(product: Product) {
  if (!confirm(`“${product.name}” katalogdan silinsin mi?`)) return
  try {
    await deleteProduct(product.id)
    await Promise.all([loadProducts(), refreshCatalogCounts()])
    showNotice('success', 'Ürün katalogdan kaldırıldı.')
  } catch (error: any) {
    showNotice('error', apiError(error, 'Ürün silinemedi.'))
  }
}

async function refreshCatalogCounts() {
  catalogs.value = await listCatalogs(account.value)
}

function fmtPrice(product: Product) {
  if (!product.price || product.price <= 0) return 'Fiyat belirtilmedi'
  return new Intl.NumberFormat('tr-TR', { style: 'currency', currency: product.currency || 'TRY' }).format(product.price / 100)
}

function previewPrice() {
  if (!pForm.value.price || pForm.value.price <= 0) return ''
  return new Intl.NumberFormat('tr-TR', { style: 'currency', currency: pForm.value.currency || 'TRY' }).format(Number(pForm.value.price) || 0)
}

function initials(value: string) {
  return value.split(/\s+/).slice(0, 2).map((part) => part.charAt(0)).join('').toUpperCase() || 'Ü'
}

function imageFailed(event: Event) {
  ;(event.target as HTMLImageElement).style.display = 'none'
}

function showNotice(type: 'success' | 'error', text: string) {
  notice.value = { type, text }
  window.setTimeout(() => {
    if (notice.value?.text === text) notice.value = null
  }, 5000)
}

function apiError(error: any, fallback: string) {
  return error?.response?.data?.message || error?.response?.data?.error?.message || fallback
}

onMounted(initialize)
onBeforeUnmount(clearProductImageSelection)
</script>

<template>
  <div class="page catalog-page">
    <header class="page-head">
      <div>
        <span class="eyebrow">WhatsApp Commerce</span>
        <h1>Ürün Kataloğu</h1>
        <p class="muted">Ürünlerinizi tek yerden yönetin ve WhatsApp görüşmelerinde paylaşmaya hazır tutun.</p>
      </div>
      <div class="head-actions">
        <label class="account-select">
          <span class="status-dot active"></span>
          <select v-model="account" @change="changeAccount">
            <option value="" disabled>WhatsApp hesabı seçin</option>
            <option v-for="item in accounts" :key="item.name" :value="item.name">{{ item.name }}</option>
          </select>
        </label>
        <button class="sync-btn" :disabled="syncing || !account" @click="doSync">
          <span :class="{ spinning: syncing }">↻</span>{{ syncing ? 'Eşitleniyor' : 'Meta ile eşitle' }}
        </button>
        <button class="primary" :disabled="!account || !catalogConfigured" @click="showCatalogModal = true">＋ Yeni katalog</button>
      </div>
    </header>

    <div v-if="notice" :class="['notice', notice.type]">
      <span>{{ notice.type === 'success' ? '✓' : '!' }}</span>{{ notice.text }}
      <button @click="notice = null">×</button>
    </div>

    <section v-if="account && !catalogConfigured" class="catalog-setup">
      <div class="setup-icon">⚙</div>
      <div class="setup-copy">
        <b>{{ account }} için katalog bağlantısını tamamlayın</b>
        <span>WABA Business ID’den farklı olarak, bu hesabın sahibi olan <strong>Meta Business Portfolio ID</strong> gereklidir.</span>
      </div>
      <input v-model="catalogBusinessId" inputmode="numeric" placeholder="Business Portfolio ID" @keyup.enter="saveCatalogSettings" />
      <button class="primary" :disabled="savingCatalogSettings || !catalogBusinessId.trim()" @click="saveCatalogSettings">
        {{ savingCatalogSettings ? 'Kaydediliyor…' : 'Katalog ayarını kaydet' }}
      </button>
    </section>

    <section class="summary-strip">
      <div class="summary-main">
        <div class="commerce-icon">▣</div>
        <div><b>{{ account || 'Hesap seçilmedi' }}</b><span>Cloud API ürün yönetimi</span></div>
      </div>
      <div class="summary-stat"><b>{{ catalogs.length }}</b><span>Katalog</span></div>
      <div class="summary-stat"><b>{{ totalProducts }}</b><span>Toplam ürün</span></div>
      <div class="summary-stat"><b>Aktif</b><span>Meta bağlantısı</span></div>
    </section>

    <div v-if="loading" class="workspace-loading">
      <span class="loader"></span><b>Katalog hazırlanıyor</b><small>Seçili WhatsApp hesabındaki ürünler getiriliyor…</small>
    </div>

    <div v-else-if="!catalogs.length" class="empty-state card">
      <div class="empty-illustration">🛍</div>
      <h2>İlk kataloğunuzu oluşturun</h2>
      <p>WhatsApp üzerinden göstermek istediğiniz ürünleri düzenli ve şık bir vitrinde toplayın.</p>
      <button class="primary" :disabled="!catalogConfigured" @click="showCatalogModal = true">Katalog oluşturmaya başla</button>
    </div>

    <div v-else class="catalog-workspace">
      <aside class="catalog-list card">
        <div class="list-head">
          <div><b>Kataloglar</b><small>{{ catalogs.length }} koleksiyon</small></div>
          <button class="icon-btn soft" title="Yeni katalog" @click="showCatalogModal = true">＋</button>
        </div>
        <button
          v-for="catalog in catalogs"
          :key="catalog.id"
          :class="['catalog-row', { active: catalog.id === selectedCatalogId }]"
          @click="selectCatalog(catalog)"
        >
          <span class="catalog-avatar">{{ initials(catalog.name) }}</span>
          <span class="catalog-copy"><b>{{ catalog.name }}</b><small>{{ catalog.product_count }} ürün</small></span>
          <span>›</span>
        </button>
      </aside>

      <main class="product-panel card">
        <div class="product-head">
          <div>
            <span class="catalog-state"><i></i> Meta ile bağlı</span>
            <h2>{{ selectedCatalog?.name }}</h2>
            <p class="muted">{{ selectedCatalog?.product_count || 0 }} ürün · {{ selectedCatalog?.whatsapp_account }}</p>
          </div>
          <div class="product-actions">
            <button class="danger-btn subtle-delete" title="Kataloğu sil" @click="selectedCatalog && removeCatalog(selectedCatalog)">Sil</button>
            <button :disabled="activating" @click="activateSelectedCatalog">{{ activating ? 'Bağlanıyor…' : 'WhatsApp’ta göster' }}</button>
            <button class="primary" @click="openNewProduct">＋ Ürün ekle</button>
          </div>
        </div>

        <div class="product-toolbar">
          <div class="product-search"><span>⌕</span><input v-model="search" placeholder="Ürün adı, açıklama veya stok kodu ara" /></div>
          <span class="result-count">{{ filteredProducts.length }} ürün gösteriliyor</span>
        </div>

        <div v-if="loadingProducts" class="product-loading"><span class="loader"></span>Ürünler getiriliyor…</div>
        <div v-else-if="!products.length" class="products-empty">
          <div class="empty-product-icon">＋</div>
          <h3>Bu katalog henüz boş</h3>
          <p>İlk ürününüzü ekleyerek WhatsApp vitrininizi oluşturmaya başlayın.</p>
          <button class="soft" @click="openNewProduct">İlk ürünü ekle</button>
        </div>
        <div v-else-if="!filteredProducts.length" class="products-empty compact">
          <h3>Aramanızla eşleşen ürün yok</h3><p>Başka bir ürün adı veya stok kodu deneyin.</p>
        </div>

        <div v-else class="product-grid">
          <article v-for="product in filteredProducts" :key="product.id" class="product-card">
            <div class="product-image">
              <div class="image-fallback">{{ initials(product.name) }}</div>
              <img v-if="product.image_url" :src="product.image_url" :alt="product.name" @error="imageFailed" />
              <span :class="['active-label', { hidden: !product.is_active }]">{{ product.is_active ? 'Yayında' : 'Meta’da gizli' }}</span>
              <div class="card-menu">
                <button class="icon-btn" title="Düzenle" @click="openEditProduct(product)">✎</button>
                <button class="icon-btn delete" title="Sil" @click="removeProduct(product)">⌫</button>
              </div>
            </div>
            <div class="product-content">
              <div class="product-title"><h3>{{ product.name }}</h3><b>{{ fmtPrice(product) }}</b></div>
              <p>{{ product.description || 'Ürün açıklaması eklenmemiş.' }}</p>
              <div class="product-meta">
                <span v-if="product.retailer_id">SKU: {{ product.retailer_id }}</span>
                <span v-else>Stok kodu yok</span>
                <a v-if="product.url" :href="product.url" target="_blank" rel="noopener">Ürün sayfası ↗</a>
              </div>
            </div>
          </article>
        </div>
      </main>
    </div>

    <div v-if="showCatalogModal" class="modal-backdrop" @click.self="showCatalogModal = false">
      <form class="modal-card catalog-modal" @submit.prevent="addCatalog">
        <div class="modal-head">
          <div><span class="eyebrow">Yeni koleksiyon</span><h2>Katalog oluştur</h2><p class="muted">Ürünlerinizi bir tema altında gruplayın.</p></div>
          <button type="button" class="icon-btn" @click="showCatalogModal = false">×</button>
        </div>
        <div class="modal-body">
          <label class="field"><span>Katalog adı</span><input v-model="newCatalogName" autofocus maxlength="80" placeholder="Ör. B2B Enerji Ürünleri" /></label>
          <div class="account-confirm"><span class="status-dot active"></span><div><b>{{ account }}</b><small>Bu WhatsApp hesabına bağlanacak</small></div></div>
        </div>
        <div class="modal-actions"><button type="button" @click="showCatalogModal = false">Vazgeç</button><button class="primary" :disabled="creatingCatalog || !newCatalogName.trim()">{{ creatingCatalog ? 'Oluşturuluyor…' : 'Kataloğu oluştur' }}</button></div>
      </form>
    </div>

    <div v-if="showProductModal" class="modal-backdrop" @click.self="showProductModal = false">
      <form class="modal-card product-modal" @submit.prevent="saveProduct">
        <div class="modal-head">
          <div><span class="eyebrow">{{ editingProduct ? 'Ürün düzenleme' : 'Yeni ürün' }}</span><h2>{{ editingProduct ? editingProduct.name : 'Kataloğa ürün ekle' }}</h2><p class="muted">Bilgiler Meta kataloğunuzla eş zamanlı güncellenir.</p></div>
          <button type="button" class="icon-btn" @click="showProductModal = false">×</button>
        </div>
        <div class="product-form-layout">
          <div class="modal-body form-side">
            <label class="field"><span>Ürün adı *</span><input v-model="pForm.name" maxlength="120" placeholder="Ürünün görünen adı" /></label>
            <label class="field"><span>Açıklama</span><textarea v-model="pForm.description" rows="3" maxlength="500" placeholder="Ürünün öne çıkan özellikleri…"></textarea></label>
            <div class="form-row">
              <label class="field price-field"><span>Fiyat (isteğe bağlı)</span><input v-model.number="pForm.price" type="number" min="0" step="0.01" placeholder="Fiyat göstermeyin" /></label>
              <label class="field currency-field"><span>Para birimi</span><select v-model="pForm.currency"><option>TRY</option><option>USD</option><option>EUR</option><option>GBP</option></select></label>
            </div>
            <label class="field image-upload-field">
              <span>Ürün görseli *</span>
              <input type="file" accept="image/jpeg,image/png,image/webp" @change="selectProductImage" />
              <small>{{ productImageFile?.name || (pForm.image_url ? 'Mevcut görsel kullanılacak; değiştirmek için yeni dosya seçin.' : 'JPG, PNG veya WEBP · en fazla 8 MB') }}</small>
            </label>
            <div class="form-row">
              <label class="field"><span>Stok kodu / SKU *</span><input v-model="pForm.retailer_id" placeholder="URN-001" /></label>
              <label class="field"><span>Ürün bağlantısı (isteğe bağlı)</span><input v-model="pForm.url" type="url" placeholder="https://…" /></label>
            </div>
            <div class="form-row">
              <label class="field"><span>Stok durumu</span><select v-model="pForm.availability"><option value="in stock">Stokta</option><option value="out of stock">Stokta yok</option></select></label>
              <label class="field"><span>Ürün durumu</span><select v-model="pForm.condition"><option value="new">Yeni</option><option value="used">Kullanılmış</option><option value="refurbished">Yenilenmiş</option></select></label>
            </div>
            <p v-if="productError" class="form-error">! {{ productError }}</p>
          </div>
          <aside class="preview-side">
            <span>WHATSAPP ÖNİZLEME</span>
            <div class="phone-preview">
              <div class="preview-image">
                <div>{{ initials(pForm.name || 'Ürün') }}</div>
                <img v-if="productImagePreview || pForm.image_url" :src="productImagePreview || pForm.image_url" alt="Önizleme" @error="imageFailed" />
              </div>
              <div class="preview-copy"><b>{{ pForm.name || 'Ürün adı' }}</b><strong v-if="previewPrice()">{{ previewPrice() }}</strong><p>{{ pForm.description || 'Ürün açıklamanız burada görünecek.' }}</p></div>
              <div class="preview-action">Ürünü görüntüle</div>
            </div>
          </aside>
        </div>
        <div class="modal-actions"><button type="button" @click="showProductModal = false; clearProductImageSelection()">Vazgeç</button><button class="primary" :disabled="savingProduct">{{ uploadingProductImage ? 'Görsel yükleniyor…' : savingProduct ? 'Kaydediliyor…' : editingProduct ? 'Değişiklikleri kaydet' : 'Ürünü yayınla' }}</button></div>
      </form>
    </div>
  </div>
</template>

<style scoped>
.catalog-page { padding: 30px 34px 44px; max-width: 1500px; margin: 0 auto; }
.page-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 24px; margin-bottom: 22px; }
.page-head h1 { margin: 3px 0 4px; }
.page-head p { margin: 0; max-width: 640px; }
.head-actions { display: flex; align-items: center; gap: 9px; flex-wrap: wrap; justify-content: flex-end; }
.account-select { display: flex; align-items: center; gap: 4px; min-height: 38px; padding-left: 11px; border: 1px solid var(--border-strong); border-radius: var(--radius-sm); background: var(--panel); }
.account-select select { width: 128px; min-height: 36px; padding: 6px 26px 6px 6px; border: 0; box-shadow: none; font-weight: 650; background: transparent; }
.sync-btn { display: flex; align-items: center; gap: 7px; }
.spinning { display: inline-block; animation: spin .8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.notice { display: flex; align-items: center; gap: 10px; margin-bottom: 15px; padding: 11px 14px; border-radius: 12px; border: 1px solid; animation: modal-in .2s ease-out; }
.notice > span { width: 23px; height: 23px; display: grid; place-items: center; border-radius: 50%; font-weight: 800; }
.notice button { margin-left: auto; min-height: auto; padding: 2px 7px; border: 0; background: transparent; font-size: 18px; }
.notice.success { color: #087a55; background: #edfbf5; border-color: #bcebd7; }.notice.success > span { color: #fff; background: #16a66f; }
.notice.error { color: #b83c3c; background: #fff4f4; border-color: #f3cccc; }.notice.error > span { color: #fff; background: var(--danger); }
.catalog-setup { display: grid; grid-template-columns: auto minmax(260px,1fr) minmax(210px,310px) auto; align-items: center; gap: 14px; margin-bottom: 16px; padding: 15px 17px; border: 1px solid #f0dca5; border-radius: 15px; background: linear-gradient(110deg,#fffaf0,#fffdf8); box-shadow: var(--shadow-sm); }
.setup-icon { width: 39px; height: 39px; display: grid; place-items: center; border-radius: 12px; color: #9a6500; background: #fff0c7; font-size: 18px; }
.setup-copy { display: flex; flex-direction: column; gap: 2px; }.setup-copy span { color: var(--muted); font-size: 11px; line-height: 1.4; }.catalog-setup input { min-height: 40px; background: #fff; }
.summary-strip { display: flex; align-items: stretch; min-height: 82px; margin-bottom: 18px; border: 1px solid var(--border); border-radius: var(--radius-lg); background: linear-gradient(110deg,#fff 60%,#f0fbf6); box-shadow: var(--shadow-sm); overflow: hidden; }
.summary-main { display: flex; align-items: center; gap: 13px; flex: 1; padding: 16px 20px; }
.commerce-icon { width: 45px; height: 45px; display: grid; place-items: center; border-radius: 14px; color: #fff; background: linear-gradient(145deg,#25d366,#0b9567); font-size: 22px; box-shadow: 0 6px 14px rgba(11,149,103,.2); }
.summary-main div:last-child, .summary-stat { display: flex; flex-direction: column; }.summary-main b { font-size: 15px; }.summary-main span, .summary-stat span { color: var(--muted); font-size: 11px; margin-top: 2px; }
.summary-stat { min-width: 130px; justify-content: center; padding: 14px 24px; border-left: 1px solid var(--border); }.summary-stat b { font-size: 17px; }
.catalog-workspace { display: grid; grid-template-columns: 255px minmax(0,1fr); gap: 16px; align-items: start; }
.catalog-list { padding: 10px; position: sticky; top: 20px; }
.list-head { display: flex; align-items: center; justify-content: space-between; padding: 8px 8px 12px; }.list-head > div { display: flex; flex-direction: column; }.list-head small { color: var(--muted); font-size: 11px; }
.catalog-row { width: 100%; min-height: 62px; display: flex; align-items: center; gap: 10px; padding: 9px; border: 0; border-radius: 12px; text-align: left; background: transparent; box-shadow: none; }
.catalog-row:hover { background: var(--bg-2); box-shadow: none; }.catalog-row.active { color: var(--brand); background: var(--brand-soft); }
.catalog-avatar { width: 39px; height: 39px; display: grid; place-items: center; flex-shrink: 0; border-radius: 11px; color: var(--brand); background: #dff7ec; font-size: 12px; font-weight: 800; }
.catalog-copy { flex: 1; min-width: 0; display: flex; flex-direction: column; }.catalog-copy b { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.catalog-copy small { color: var(--muted); font-size: 11px; }
.product-panel { min-height: 500px; padding: 0; overflow: hidden; }
.product-head { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 23px 25px 18px; }.product-head h2 { margin: 4px 0 1px; font-size: 21px; }.product-head p { margin: 0; }
.catalog-state { color: #07865c; font-size: 11px; font-weight: 650; }.catalog-state i { display: inline-block; width: 7px; height: 7px; margin-right: 5px; border-radius: 50%; background: #20c77a; }
.product-actions { display: flex; gap: 8px; }.subtle-delete { border-color: transparent; }
.product-toolbar { display: flex; align-items: center; gap: 12px; padding: 13px 25px; border-top: 1px solid var(--border); border-bottom: 1px solid var(--border); background: var(--bg-2); }
.product-search { max-width: 450px; flex: 1; display: flex; align-items: center; padding-left: 12px; border: 1px solid var(--border-strong); border-radius: 11px; background: #fff; }.product-search span { color: var(--muted); font-size: 20px; }.product-search input { min-height: 38px; border: 0; box-shadow: none; background: transparent; }
.result-count { margin-left: auto; color: var(--muted); font-size: 11px; }
.product-grid { display: grid; grid-template-columns: repeat(auto-fill,minmax(245px,1fr)); gap: 15px; padding: 21px 25px 28px; }
.product-card { min-width: 0; overflow: hidden; border: 1px solid var(--border); border-radius: 16px; background: #fff; transition: transform .18s ease,box-shadow .18s ease,border-color .18s ease; }.product-card:hover { transform: translateY(-3px); border-color: #cbd6d2; box-shadow: var(--shadow); }
.product-image { position: relative; height: 180px; overflow: hidden; background: linear-gradient(145deg,#eff4f2,#e5eeea); }.product-image img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }.image-fallback { width: 100%; height: 100%; display: grid; place-items: center; color: #88a399; font-size: 32px; font-weight: 800; }
.active-label { position: absolute; left: 10px; top: 10px; z-index: 2; padding: 4px 8px; border-radius: 999px; color: #087a55; background: rgba(238,255,247,.94); box-shadow: 0 2px 8px rgba(0,0,0,.08); font-size: 10px; font-weight: 700; }
.active-label.hidden { color: #8a4b08; background: rgba(255,247,230,.96); }
.card-menu { position: absolute; right: 9px; top: 9px; z-index: 2; display: flex; gap: 6px; opacity: 0; transform: translateY(-3px); transition: .16s ease; }.product-card:hover .card-menu { opacity: 1; transform: none; }.card-menu .icon-btn { min-height: 34px; width: 34px; border: 0; background: rgba(255,255,255,.94); box-shadow: 0 2px 9px rgba(0,0,0,.1); }.card-menu .delete { color: var(--danger); }
.product-content { padding: 14px 15px 15px; }.product-title { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; }.product-title h3 { min-width: 0; margin: 0; font-size: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.product-title b { color: var(--brand); white-space: nowrap; font-size: 13px; }.product-content > p { height: 34px; margin: 6px 0 11px; color: var(--muted); font-size: 11px; line-height: 1.45; overflow: hidden; }.product-meta { display: flex; justify-content: space-between; gap: 8px; padding-top: 10px; border-top: 1px solid var(--border); color: #829096; font-size: 10px; }.product-meta a { white-space: nowrap; }
.products-empty,.workspace-loading { min-height: 390px; display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 36px; text-align: center; }.products-empty h3 { margin: 12px 0 4px; }.products-empty p { max-width: 370px; margin: 0 0 15px; color: var(--muted); }.products-empty.compact { min-height: 260px; }.empty-product-icon { width: 62px; height: 62px; display: grid; place-items: center; border: 1px dashed #9dceb9; border-radius: 18px; color: var(--brand); background: var(--brand-soft); font-size: 25px; }
.workspace-loading { min-height: 440px; gap: 6px; }.workspace-loading small { color: var(--muted); }.loader { width: 24px; height: 24px; margin-bottom: 8px; border: 3px solid #dce8e3; border-top-color: var(--brand); border-radius: 50%; animation: spin .8s linear infinite; }.product-loading { display: flex; align-items: center; justify-content: center; gap: 10px; min-height: 300px; color: var(--muted); }.product-loading .loader { margin: 0; }
.empty-state { min-height: 430px; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; }.empty-state h2 { margin: 16px 0 4px; }.empty-state p { max-width: 460px; margin: 0 0 18px; color: var(--muted); }.empty-illustration { width: 86px; height: 86px; display: grid; place-items: center; border-radius: 25px; background: var(--brand-soft); font-size: 38px; }
.field { display: flex; flex-direction: column; gap: 6px; }.field > span { color: #46545a; font-size: 12px; font-weight: 650; }.catalog-modal { max-width: 500px; }.catalog-modal .modal-head p,.product-modal .modal-head p { margin: 0; font-size: 12px; }.account-confirm { display: flex; align-items: center; gap: 10px; margin-top: 14px; padding: 12px; border-radius: 12px; background: var(--bg-2); }.account-confirm div { display: flex; flex-direction: column; }.account-confirm small { color: var(--muted); }
.image-upload-field small { color: var(--muted); font-size: 10px; }
.product-modal { width: min(880px,100%); }.product-form-layout { display: grid; grid-template-columns: minmax(0,1fr) 270px; }.form-side { display: flex; flex-direction: column; gap: 13px; }.form-row { display: flex; gap: 12px; }.form-row .field { flex: 1; }.currency-field { max-width: 120px; }.preview-side { padding: 22px; border-left: 1px solid var(--border); background: #f2f6f4; }.preview-side > span { display: block; margin-bottom: 10px; color: var(--muted); font-size: 9px; font-weight: 750; letter-spacing: .08em; }.phone-preview { overflow: hidden; border-radius: 13px; background: #fff; box-shadow: 0 8px 26px rgba(17,45,35,.1); }.preview-image { position: relative; height: 170px; background: #e7efec; }.preview-image > div { height: 100%; display: grid; place-items: center; color: #8ba79c; font-size: 28px; font-weight: 800; }.preview-image img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }.preview-copy { display: flex; flex-direction: column; padding: 13px 14px; }.preview-copy strong { margin-top: 4px; color: var(--brand); }.preview-copy p { min-height: 32px; margin: 6px 0 0; color: var(--muted); font-size: 10px; }.preview-action { padding: 10px; border-top: 1px solid var(--border); color: #1685b8; text-align: center; font-size: 11px; font-weight: 650; }.form-error { margin: 0; padding: 9px 11px; border-radius: 10px; color: var(--danger); background: var(--danger-soft); font-size: 12px; }
@media (max-width: 1000px) { .catalog-page { padding: 24px; }.page-head { flex-direction: column; }.head-actions { width: 100%; justify-content: flex-start; }.catalog-setup { grid-template-columns: auto 1fr; }.catalog-setup input { grid-column: 1/2; }.catalog-setup button { grid-column: 2/3; }.summary-stat { min-width: 110px; padding: 12px 15px; }.catalog-workspace { grid-template-columns: 220px minmax(0,1fr); }.product-grid { grid-template-columns: repeat(auto-fill,minmax(220px,1fr)); } }
@media (max-width: 760px) { .catalog-page { padding: 16px 13px 30px; }.head-actions { display: grid; grid-template-columns: 1fr 1fr; }.account-select { grid-column: 1/-1; }.account-select select { flex: 1; width: auto; }.summary-strip { flex-wrap: wrap; }.summary-main { width: 100%; flex-basis: 100%; }.summary-stat { flex: 1; min-width: 33%; padding: 11px; border-top: 1px solid var(--border); }.catalog-workspace { display: block; }.catalog-list { position: static; display: flex; gap: 8px; overflow-x: auto; margin-bottom: 12px; padding: 9px; }.list-head { min-width: 110px; }.list-head .icon-btn { display: none; }.catalog-row { min-width: 190px; }.product-head { align-items: flex-start; padding: 18px; }.product-actions { flex-direction: column-reverse; }.product-toolbar { padding: 11px 14px; }.result-count { display: none; }.product-grid { grid-template-columns: repeat(2,minmax(0,1fr)); padding: 13px; gap: 10px; }.product-image { height: 135px; }.product-title { flex-direction: column; }.product-content { padding: 11px; }.card-menu { opacity: 1; transform: none; }.product-form-layout { display: block; }.preview-side { display: none; }.form-row { flex-direction: column; }.currency-field { max-width: none; } }
@media (max-width: 460px) { .product-grid { grid-template-columns: 1fr; }.product-image { height: 190px; }.summary-stat:last-child { display: none; } }
</style>
