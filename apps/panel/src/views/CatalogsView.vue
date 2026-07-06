<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  listCatalogs,
  createCatalog,
  syncCatalogs,
  deleteCatalog,
  listProducts,
  createProduct,
  type Catalog,
  type Product
} from '@/services/catalog'
import { listAccounts, type Account } from '@/services/campaigns'

const accounts = ref<Account[]>([])
const account = ref('')
const catalogs = ref<Catalog[]>([])
const loading = ref(false)
const msg = ref('')

// new catalog
const newCatalogName = ref('')
const creatingCatalog = ref(false)
const syncing = ref(false)

// products per catalog
const openCatalog = ref<string | null>(null)
const products = ref<Product[]>([])
const loadingProducts = ref(false)

// new product
const showProductForm = ref(false)
const pForm = ref(blankProduct())
const savingProduct = ref(false)

function blankProduct() {
  return { name: '', description: '', price: 0, currency: 'TRY', image_url: '', url: '', retailer_id: '' }
}

async function loadAll() {
  loading.value = true
  try {
    if (!accounts.value.length) accounts.value = await listAccounts()
    if (!account.value && accounts.value.length) account.value = accounts.value[0].name
    catalogs.value = await listCatalogs(account.value)
  } finally {
    loading.value = false
  }
}

async function addCatalog() {
  if (!newCatalogName.value.trim() || !account.value) return
  creatingCatalog.value = true
  msg.value = ''
  try {
    await createCatalog(account.value, newCatalogName.value.trim())
    newCatalogName.value = ''
    await loadAll()
  } catch (e: any) {
    msg.value = '✗ ' + (e?.response?.data?.message || 'Katalog oluşturulamadı.')
  } finally {
    creatingCatalog.value = false
  }
}

async function doSync() {
  if (!account.value) return
  syncing.value = true
  msg.value = ''
  try {
    await syncCatalogs(account.value)
    msg.value = '✓ Meta’dan senkronlandı.'
    await loadAll()
  } catch (e: any) {
    msg.value = '✗ ' + (e?.response?.data?.message || 'Senkronlanamadı.')
  } finally {
    syncing.value = false
  }
}

async function removeCatalog(c: Catalog) {
  if (!confirm(`"${c.name}" kataloğu silinsin mi?`)) return
  await deleteCatalog(c.id)
  await loadAll()
}

async function toggleProducts(c: Catalog) {
  if (openCatalog.value === c.id) {
    openCatalog.value = null
    return
  }
  openCatalog.value = c.id
  showProductForm.value = false
  loadingProducts.value = true
  try {
    products.value = await listProducts(c.id)
  } finally {
    loadingProducts.value = false
  }
}

async function addProduct(c: Catalog) {
  if (!pForm.value.name.trim() || pForm.value.price <= 0) {
    alert('Ürün adı ve fiyat gerekli.')
    return
  }
  savingProduct.value = true
  try {
    await createProduct(c.id, { ...pForm.value })
    pForm.value = blankProduct()
    showProductForm.value = false
    products.value = await listProducts(c.id)
    await loadAll()
  } catch (e: any) {
    alert(e?.response?.data?.message || 'Ürün eklenemedi.')
  } finally {
    savingProduct.value = false
  }
}

function fmtPrice(p: Product) {
  return (p.price / 100).toLocaleString('tr-TR') + ' ' + p.currency
}

onMounted(loadAll)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1>Katalog</h1>
        <p class="muted">WhatsApp ürün katalogları. Sohbette ürün paylaşımı için Meta’da yayınlanır.</p>
      </div>
    </header>

    <div class="card toolbar">
      <select v-model="account" @change="loadAll">
        <option value="" disabled>Kanal seçin</option>
        <option v-for="a in accounts" :key="a.name" :value="a.name">{{ a.name }}</option>
      </select>
      <input v-model="newCatalogName" placeholder="Yeni katalog adı" @keyup.enter="addCatalog" />
      <button class="primary" :disabled="creatingCatalog" @click="addCatalog">＋ Katalog</button>
      <button :disabled="syncing" @click="doSync">{{ syncing ? '…' : '↻ Senkronla' }}</button>
      <span v-if="msg" class="muted small">{{ msg }}</span>
    </div>

    <div v-if="loading" class="muted center">Yükleniyor…</div>
    <div v-else-if="!catalogs.length" class="card center muted">Katalog yok. Yukarıdan oluşturun veya senkronlayın.</div>

    <div v-for="c in catalogs" :key="c.id" class="card catalog">
      <div class="cat-head">
        <div>
          <b>{{ c.name }}</b>
          <span class="muted small">· {{ c.product_count }} ürün · {{ c.whatsapp_account }}</span>
        </div>
        <div class="cat-actions">
          <button @click="toggleProducts(c)">{{ openCatalog === c.id ? 'Gizle' : 'Ürünler' }}</button>
          <button class="danger-btn" @click="removeCatalog(c)">Sil</button>
        </div>
      </div>

      <div v-if="openCatalog === c.id" class="products">
        <div v-if="loadingProducts" class="muted small">Yükleniyor…</div>
        <div v-else>
          <div v-for="p in products" :key="p.id" class="product">
            <img v-if="p.image_url" :src="p.image_url" class="p-img" />
            <div class="p-info">
              <b>{{ p.name }}</b>
              <span class="muted small">{{ fmtPrice(p) }}</span>
              <div class="muted small">{{ p.description }}</div>
            </div>
          </div>
          <div v-if="!products.length" class="muted small">Ürün yok.</div>

          <button v-if="!showProductForm" class="add-p" @click="showProductForm = true">＋ Ürün Ekle</button>
          <div v-else class="p-form">
            <div class="row">
              <input v-model="pForm.name" placeholder="Ürün adı *" />
              <input v-model.number="pForm.price" type="number" step="0.01" placeholder="Fiyat *" />
              <input v-model="pForm.currency" placeholder="TRY" class="cur" />
            </div>
            <input v-model="pForm.description" placeholder="Açıklama" />
            <div class="row">
              <input v-model="pForm.image_url" placeholder="Görsel URL" />
              <input v-model="pForm.retailer_id" placeholder="SKU / stok kodu" />
            </div>
            <div class="form-actions">
              <button type="button" @click="showProductForm = false">İptal</button>
              <button class="primary" :disabled="savingProduct" @click="addProduct(c)">Kaydet</button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page { padding: 24px; max-width: 860px; }
.page-head { margin-bottom: 16px; }
.page-head h1 { margin: 0 0 4px; font-size: 20px; }
.page-head p { margin: 0; }
.center { text-align: center; padding: 24px; }
.toolbar { display: flex; gap: 8px; align-items: center; margin-bottom: 16px; flex-wrap: wrap; }
.toolbar select, .toolbar input { max-width: 220px; }

.catalog { margin-bottom: 10px; }
.cat-head { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
.cat-actions { display: flex; gap: 8px; }
.danger-btn { color: var(--danger); }

.products { margin-top: 12px; padding-top: 12px; border-top: 1px solid var(--border); }
.product { display: flex; gap: 10px; padding: 8px 0; border-bottom: 1px solid var(--border); }
.p-img { width: 48px; height: 48px; object-fit: cover; border-radius: 6px; }
.p-info { display: flex; flex-direction: column; gap: 2px; }
.add-p { margin-top: 10px; }
.p-form { margin-top: 10px; display: flex; flex-direction: column; gap: 8px; }
.p-form .row { display: flex; gap: 8px; }
.p-form .row input { flex: 1; }
.p-form .cur { max-width: 80px; }
.form-actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
