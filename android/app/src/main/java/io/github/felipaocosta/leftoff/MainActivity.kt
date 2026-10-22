package io.github.felipaocosta.leftoff

import android.Manifest
import android.annotation.SuppressLint
import android.content.Intent
import android.content.res.Configuration
import android.graphics.Color
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.view.Gravity
import android.view.View
import android.webkit.JavascriptInterface
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.FrameLayout
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.OnBackPressedCallback
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.splashscreen.SplashScreen.Companion.installSplashScreen
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.swiperefreshlayout.widget.SwipeRefreshLayout
import java.io.File
import kotlin.concurrent.thread
import io.github.felipaocosta.leftoff.mobile.Mobile

/**
 * The Android shell (Phase 7): a WebView over the Go core (mobile/), which
 * runs the same server as the other builds on 127.0.0.1. Chapter links to
 * other sites open in the browser; the app never shows chapter pages.
 */
class MainActivity : ComponentActivity() {
    private lateinit var web: WebView
    private lateinit var root: FrameLayout
    private lateinit var bottomBar: View
    private lateinit var pull: SwipeRefreshLayout
    /** The splash screen stays until the first page is drawn (or start fails). */
    @Volatile private var loading = true
    private var localUrl = ""
    private var targetUrl = ""
    private var fileCallback: ValueCallback<Array<Uri>>? = null

    private val pickFile = registerForActivityResult(ActivityResultContracts.GetContent()) { uri ->
        fileCallback?.onReceiveValue(if (uri != null) arrayOf(uri) else null)
        fileCallback = null
    }

    private val askNotifications = registerForActivityResult(ActivityResultContracts.RequestPermission()) {}

    /** The export waiting for the user to choose where to save it. */
    private var pendingExport: ByteArray? = null

    private val createFile = registerForActivityResult(ActivityResultContracts.CreateDocument("application/json")) { uri ->
        val data = pendingExport
        pendingExport = null
        if (uri == null || data == null) {
            reportSaved(ok = false, canceled = true)
            return@registerForActivityResult
        }
        try {
            contentResolver.openOutputStream(uri)?.use { it.write(data) } ?: error("cannot write")
            val name = uri.lastPathSegment?.substringAfterLast(':') ?: ""
            Toast.makeText(this, getString(R.string.saved_file, name), Toast.LENGTH_LONG).show()
            reportSaved(ok = true, name = name)
        } catch (e: Exception) {
            Toast.makeText(this, getString(R.string.save_failed, e.message), Toast.LENGTH_LONG).show()
            reportSaved(ok = false, error = e.message ?: "")
        }
    }

    /** Answers the web UI's export with a "mt-file-saved" event. */
    private fun reportSaved(ok: Boolean, canceled: Boolean = false, name: String = "", error: String = "") {
        val detail = org.json.JSONObject()
            .put("ok", ok).put("canceled", canceled).put("name", name).put("error", error)
        web.evaluateJavascript(
            "window.dispatchEvent(new CustomEvent('mt-file-saved', {detail: $detail}))", null,
        )
    }

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        installSplashScreen().setKeepOnScreenCondition { loading }
        super.onCreate(savedInstanceState)
        web = WebView(this)
        pull = SwipeRefreshLayout(this).apply {
            addView(web)
            // Only when the page is at its top; the web UI decides what a pull does.
            setOnChildScrollUpCallback { _, _ -> web.scrollY > 0 }
            setOnRefreshListener { pullRefresh() }
        }
        setContentView(edgeToEdge(pull))
        applyTheme(isSystemDark())
        web.settings.javaScriptEnabled = true
        web.settings.domStorageEnabled = true
        web.addJavascriptInterface(Bridge(), "LeftOffAndroid")
        web.webViewClient = Client()
        web.webChromeClient = Chrome()
        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                if (web.canGoBack()) web.goBack() else finish()
            }
        })
        val dataDir = File(filesDir, "leftoff").absolutePath
        thread(name = "go-core") {
            try {
                Mobile.setBrowser(WebViewBrowser.get(this)) // JavaScript pages and anti-bot checks (RF-56)
                localUrl = Mobile.start(dataDir)
                targetUrl = Mobile.targetURL()
                runOnUiThread { web.loadUrl(targetUrl + route(intent)) }
            } catch (e: Exception) {
                loading = false
                runOnUiThread {
                    Toast.makeText(this, getString(R.string.start_failed, e.message), Toast.LENGTH_LONG).show()
                }
            }
        }
        CheckWorker.schedule(this)
        askNotificationsOnce()
    }

    /** Android 13+: asks once to show the new-chapter notifications. */
    private fun askNotificationsOnce() {
        if (Build.VERSION.SDK_INT < 33) return
        val prefs = getSharedPreferences("app", MODE_PRIVATE)
        if (prefs.getBoolean("asked_notifications", false)) return
        prefs.edit().putBoolean("asked_notifications", true).apply()
        askNotifications.launch(Manifest.permission.POST_NOTIFICATIONS)
    }

    /**
     * Pull to refresh: the screen handles a cancelable "mt-pull-refresh"
     * (dispatchEvent answers false) and calls pullRefreshDone; a screen that
     * does not handle it stops the spinner at once.
     */
    private fun pullRefresh() {
        web.evaluateJavascript(
            "window.dispatchEvent(new CustomEvent('mt-pull-refresh', {cancelable: true}))",
        ) { handled -> if (handled != "false") pull.isRefreshing = false }
    }

    /** A share or a shortcut while the app is open opens its screen. */
    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        if (targetUrl.isNotEmpty()) web.loadUrl(targetUrl + route(intent))
    }

    /**
     * The web UI route an intent asks for: a shared link opens Add with it,
     * the launcher shortcuts open Add or New chapters.
     */
    private fun route(intent: Intent?): String {
        if (intent?.action == Intent.ACTION_SEND) {
            val text = intent.getStringExtra(Intent.EXTRA_TEXT) ?: return ""
            return "#/add?url=" + Uri.encode(text)
        }
        return when (intent?.data?.takeIf { it.scheme == "leftoff" }?.host) {
            "add" -> "#/add"
            "news" -> "#/news"
            else -> ""
        }
    }

    override fun onResume() {
        super.onResume()
        WebViewBrowser.foreground = this
    }

    override fun onPause() {
        if (WebViewBrowser.foreground === this) WebViewBrowser.foreground = null
        super.onPause()
    }

    private var check: View? = null

    /**
     * Shows the core's browser tab over the app so the user can pass an
     * anti-bot check on host (RF-56); cancel leaves the page, which ends the
     * wait. Called on the UI thread by WebViewBrowser.
     */
    fun showCheck(tab: WebView, host: String, cancel: () -> Unit): Boolean {
        hideCheck()
        (tab.parent as? android.view.ViewGroup)?.removeView(tab)
        val title = android.widget.TextView(this).apply {
            text = getString(R.string.check_title, host)
            setPadding(32, 24, 32, 8)
            textSize = 16f
        }
        val cancelButton = android.widget.Button(this).apply {
            text = getString(R.string.check_cancel)
            setOnClickListener { cancel() }
        }
        val panel = android.widget.LinearLayout(this).apply {
            orientation = android.widget.LinearLayout.VERTICAL
            setBackgroundColor(if (isSystemDark()) Color.parseColor("#111317") else Color.WHITE)
            isClickable = true // the app below stays untouched
            addView(title)
            addView(cancelButton, android.widget.LinearLayout.LayoutParams(
                android.widget.LinearLayout.LayoutParams.WRAP_CONTENT, android.widget.LinearLayout.LayoutParams.WRAP_CONTENT,
            ).apply { marginStart = 24 })
            addView(tab, android.widget.LinearLayout.LayoutParams(
                android.widget.LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f,
            ))
        }
        // Same place as the page: inside the system bars (edgeToEdge margins).
        root.addView(panel, FrameLayout.LayoutParams(pull.layoutParams as FrameLayout.LayoutParams))
        check = panel
        return true
    }

    /** Hosts the core's tab behind the page, where it renders but is not seen. */
    fun hostHidden(tab: WebView): Boolean {
        root.addView(tab, 0, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
        return true
    }

    /** Takes the check off screen (UI thread). */
    fun hideCheck() {
        val panel = check as? android.view.ViewGroup ?: return
        panel.removeAllViews()
        (panel.parent as? android.view.ViewGroup)?.removeView(panel)
        check = null
    }

    override fun onDestroy() {
        hideCheck()
        WebViewBrowser.detach(this)
        web.destroy()
        if (isFinishing) Mobile.stop()
        super.onDestroy()
    }

    /** Keeps the app's own pages in the WebView and sends the rest to the browser. */
    private inner class Client : WebViewClient() {
        override fun onPageCommitVisible(view: WebView, url: String) {
            loading = false
        }

        override fun onPageFinished(view: WebView, url: String) {
            loading = false
            pull.isRefreshing = false // a navigation ends any pull
        }

        override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
            val url = request.url.toString()
            if (url.startsWith(localUrl) || (targetUrl != localUrl && url.startsWith(targetUrl))) return false
            startActivity(Intent(Intent.ACTION_VIEW, request.url))
            return true
        }

        override fun onReceivedError(view: WebView, request: WebResourceRequest, error: WebResourceError) {
            // The connected server is down: fall back to this device's library.
            if (request.isForMainFrame && targetUrl != localUrl && request.url.toString().startsWith(targetUrl)) {
                Toast.makeText(this@MainActivity, R.string.server_unreachable, Toast.LENGTH_LONG).show()
                targetUrl = localUrl
                view.loadUrl(localUrl)
            }
        }
    }

    /** File inputs (import a backup, upload a cover). */
    private inner class Chrome : WebChromeClient() {
        override fun onShowFileChooser(
            view: WebView,
            callback: ValueCallback<Array<Uri>>,
            params: FileChooserParams,
        ): Boolean {
            fileCallback?.onReceiveValue(null)
            fileCallback = callback
            val type = params.acceptTypes.firstOrNull { it.isNotBlank() }?.let { if (it.startsWith(".")) "*/*" else it } ?: "*/*"
            pickFile.launch(type)
            return true
        }
    }

    /**
     * Draws behind the system bars (required from Android 15) and keeps the
     * page out of them: the status bar shows the page color, the gesture
     * area shows the bottom tab bar color, and the keyboard pushes the page up.
     */
    private fun edgeToEdge(content: View): View {
        WindowCompat.setDecorFitsSystemWindows(window, false)
        root = FrameLayout(this)
        bottomBar = View(this)
        root.addView(content, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
        root.addView(bottomBar, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, 0, Gravity.BOTTOM))
        ViewCompat.setOnApplyWindowInsetsListener(root) { _, insets ->
            val bars = insets.getInsets(WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout())
            val ime = insets.getInsets(WindowInsetsCompat.Type.ime())
            val bottom = maxOf(bars.bottom, ime.bottom)
            (content.layoutParams as FrameLayout.LayoutParams).setMargins(bars.left, bars.top, bars.right, bottom)
            content.requestLayout()
            bottomBar.layoutParams = (bottomBar.layoutParams as FrameLayout.LayoutParams).apply { height = bottom }
            WindowInsetsCompat.CONSUMED
        }
        return root
    }

    private fun isSystemDark() =
        resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK == Configuration.UI_MODE_NIGHT_YES

    /** Colors of the web theme (web/src/app.css: --bg and --surface). */
    private fun applyTheme(dark: Boolean) {
        val page = if (dark) Color.parseColor("#111317") else Color.parseColor("#f6f6f8")
        val bar = if (dark) Color.parseColor("#1a1d23") else Color.WHITE
        root.setBackgroundColor(page)
        web.setBackgroundColor(page)
        pull.setProgressBackgroundColorSchemeColor(bar)
        pull.setColorSchemeColors(if (dark) Color.parseColor("#748ffc") else Color.parseColor("#3b5bdb"))
        bottomBar.setBackgroundColor(bar)
        WindowCompat.getInsetsController(window, root).apply {
            isAppearanceLightStatusBars = !dark
            isAppearanceLightNavigationBars = !dark
        }
    }

    /** Lets the web UI save the library export, which a WebView cannot download from a blob. */
    private inner class Bridge {
        /** The web UI tells which theme is in use (it may differ from the system's). */
        @JavascriptInterface
        fun setDarkTheme(dark: Boolean) {
            runOnUiThread { applyTheme(dark) }
        }

        /** The screen finished the work of a pull to refresh. */
        @JavascriptInterface
        fun pullRefreshDone() {
            runOnUiThread { pull.isRefreshing = false }
        }

        /** The web UI's export: the user picks the folder and name (system picker). */
        @JavascriptInterface
        fun saveFile(name: String, content: String) {
            runOnUiThread {
                pendingExport = content.toByteArray()
                createFile.launch(name.substringAfterLast('/'))
            }
        }
    }
}
