package io.github.felipaocosta.leftoff

import android.annotation.SuppressLint
import android.content.Context
import android.os.Handler
import android.os.Looper
import android.webkit.CookieManager
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference
import io.github.felipaocosta.leftoff.mobile.Browser

/**
 * The WebView lent to the Go core (Mobile.setBrowser): it renders pages
 * that build their chapter list with JavaScript and passes anti-bot checks
 * (RF-56). The core decides what to do (internal/fetch/browser/driven.go);
 * this class only loads pages and runs scripts on the UI thread while the
 * calling core thread waits. When a check needs a person, show() puts the
 * tab on screen in the visible MainActivity; with the app closed it can't.
 */
class WebViewBrowser private constructor(private val app: Context) : Browser {
    private val main = Handler(Looper.getMainLooper())
    private var web: WebView? = null
    @Volatile private var pageLoaded: CountDownLatch? = null
    @Volatile private var shownIn: MainActivity? = null
    private val release = Runnable {
        val w = web ?: return@Runnable
        if (shownIn != null) return@Runnable
        (w.parent as? android.view.ViewGroup)?.removeView(w)
        w.destroy()
        web = null
    }

    companion object {
        // Holds only the application context and a WebView made from it: no
        // activity leaks through it.
        @SuppressLint("StaticFieldLeak")
        @Volatile private var instance: WebViewBrowser? = null

        fun get(context: Context): WebViewBrowser =
            instance ?: synchronized(this) {
                instance ?: WebViewBrowser(context.applicationContext).also { instance = it }
            }

        @Volatile private var visible = java.lang.ref.WeakReference<MainActivity>(null)

        /** The activity on screen, where a check can be shown (null when closed). */
        var foreground: MainActivity?
            get() = visible.get()
            set(value) {
                visible = java.lang.ref.WeakReference(value)
            }

        /** The activity is going away: take the tab out of its views (UI thread). */
        fun detach(activity: MainActivity) {
            val w = instance?.web ?: return
            if (w.context === activity || (w.parent as? android.view.View)?.context === activity) {
                (w.parent as? android.view.ViewGroup)?.removeView(w)
            }
        }

        /** A page tab is freed after this long without use (it holds tens of MB). */
        private const val IDLE_MS = 60_000L
        private const val EVAL_MS = 30_000L
    }

    override fun load(url: String, timeoutMs: Long) {
        val loaded = CountDownLatch(1)
        pageLoaded = loaded
        onMain { done: (Unit) -> Unit -> tab().loadUrl(url); done(Unit) }
        // A page that never finishes loading is read as it is, like on the desktop.
        loaded.await(timeoutMs, TimeUnit.MILLISECONDS)
    }

    override fun eval(js: String): String =
        onMain { done: (String) -> Unit -> tab().evaluateJavascript(js) { done(it ?: "null") } }

    override fun show(host: String): Boolean {
        val activity = foreground ?: return false
        return onMain { done: (Boolean) -> Unit ->
            shownIn = activity
            done(activity.showCheck(tab(), host) { tab().loadUrl("about:blank") })
        }
    }

    override fun hide() {
        onMain { done: (Unit) -> Unit ->
            shownIn?.hideCheck()
            shownIn = null
            web?.let { place(it) }
            done(Unit)
        }
    }

    override fun cookies(url: String): String = CookieManager.getInstance().getCookie(url) ?: ""

    override fun userAgent(): String = WebSettings.getDefaultUserAgent(app)

    /** The tab, created on first use; call on the UI thread. */
    @SuppressLint("SetJavaScriptEnabled")
    private fun tab(): WebView {
        main.removeCallbacks(release)
        main.postDelayed(release, IDLE_MS)
        web?.let { if (it.parent == null) place(it); return it }
        return WebView(app).also { w ->
            w.settings.javaScriptEnabled = true
            w.settings.domStorageEnabled = true
            w.webViewClient = object : WebViewClient() {
                override fun onPageFinished(view: WebView, url: String) {
                    pageLoaded?.countDown()
                }
            }
            CookieManager.getInstance().setAcceptCookie(true)
            CookieManager.getInstance().setAcceptThirdPartyCookies(w, true)
            place(w)
            web = w
        }
    }

    /**
     * Gives the tab a window when the app is open (hidden behind its page:
     * a detached WebView may throttle the scripts that load chapter lists),
     * or else a phone-sized layout.
     */
    private fun place(w: WebView) {
        val activity = foreground
        if (activity != null && w.parent == null && activity.hostHidden(w)) return
        if (w.parent == null) {
            val m = app.resources.displayMetrics
            w.layout(0, 0, m.widthPixels, m.heightPixels)
        }
    }

    /** Runs block on the UI thread and waits for its answer; never call it from there. */
    private fun <T> onMain(block: (done: (T) -> Unit) -> Unit): T {
        check(Looper.myLooper() != Looper.getMainLooper()) { "WebViewBrowser called on the UI thread" }
        val result = AtomicReference<T>()
        val latch = CountDownLatch(1)
        main.post {
            try {
                block { result.set(it); latch.countDown() }
            } catch (e: Exception) {
                latch.countDown()
            }
        }
        if (!latch.await(EVAL_MS, TimeUnit.MILLISECONDS)) error("WebView did not answer")
        return result.get() ?: error("WebView failed")
    }
}
