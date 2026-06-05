package digiflazz

// RC Classification

// Fatal - tidak bisa retry, langsung Failed
var FatalRCs = map[string]bool{
	"40": true, // Payload Error
	"41": true, // Signature tidak valid
	"42": true, // Gagal memproses API
	"44": true, // Saldo tidak cukup
	"45": true, // IP tidak dikenali
	"47": true, // Transaksi di buyer lain
	"50": true, // Transaksi tidak ditemukan
	"51": true, // Nomor diblokir
	"54": true, // Nomor salah
	"57": true, // Digit kurang/lebih
	"60": true, // Tagihan belum ada
	"61": true, // Belum deposit
	"64": true, // Tarik tiket gagal
	"67": true, // Seller belum verifikasi
	"72": true, // Unreg paket dulu
	"73": true, // Kwh melebihi batas
	"74": true, // Transaksi Refund
	"82": true, // Akun belum verifikasi
	"84": true, // Nominal tidak valid
	"87": true, // E-money bukan kelipatan 1000
}

// Retryable - switch ke SKU lain
var RetryableSwitchRCs = map[string]bool{
	"01": true, // Timeout
	"02": true, // Transaksi Gagal
	"43": true, // SKU tidak ditemukan
	"49": true, // Ref ID tidak unik (ganti ref_id suffix)
	"52": true, // Prefix tidak sesuai
	"53": true, // Produk Seller Tidak Tersedia
	"55": true, // Produk Sedang Gangguan
	"56": true, // Limit saldo seller
	"58": true, // Sedang Cut Off
	"59": true, // Luar wilayah
	"62": true, // Seller sedang gangguan
	"63": true, // Tidak support multi
	"65": true, // Limit transaksi multi
	"66": true, // Cut Off (Perbaikan Sistem)
	"68": true, // Stok habis
	"69": true, // Harga seller > harga buyer
	"70": true, // Timeout Dari Biller
	"71": true, // Produk Tidak Stabil
	"80": true, // Akun diblokir seller
	"81": true, // Seller diblokir
}

// Retryable - tunggu lalu retry (SKU sama)
var RetryableWaitRCs = map[string]bool{
	"85": true, // Limitasi transaksi (1 menit)
	"86": true, // Limitasi cek PLN
}

// Pending - tunggu callback dari Digiflazz
var PendingRCs = map[string]bool{
	"03": true, // Transaksi Pending
	"99": true, // DF Router Issue
}

// Helper functions
func IsFatal(rc string) bool {
	return FatalRCs[rc]
}

func IsRetryableSwitchSKU(rc string) bool {
	return RetryableSwitchRCs[rc]
}

func IsRetryableWait(rc string) bool {
	return RetryableWaitRCs[rc]
}

func IsRetryable(rc string) bool {
	return RetryableSwitchRCs[rc] || RetryableWaitRCs[rc]
}

func IsPending(rc string) bool {
	return PendingRCs[rc]
}

func IsSuccess(rc string) bool {
	return rc == "00"
}

func NeedsNewRefID(rc string) bool {
	return rc == "49"
}
