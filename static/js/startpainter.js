PainterCore = {
    init: function() {
        var eForm, tForm, eQr, tQr;
        (eForm = document.forms.namedItem("new-post").getElementsByClassName("painter-ctrl")[0]) &&
        (tForm = eForm.getElementsByTagName("button"))[0] &&
        (eQr = document.forms.namedItem("reply-post").getElementsByClassName("painter-ctrl")[0]) &&
        (tQr = eQr.getElementsByTagName("button"))[0] &&
                ((this.data = null),
                (this.replayBlob = null),
                (this.time = 0),
                (this.formBtnDraw = tForm[0]),
                (this.formBtnClear = tForm[1]),
                (this.formBtnFile = document.getElementById("file")),
                (this.formLblDrawing = document.getElementById("form-drawlabel")),
                //(this.formCbReplay = e.getElementsByClassName("painter-replay")[0]),
                (this.qrBtnDraw = tQr[0]),
                (this.qrBtnClear = tQr[1]),
                (this.qrBtnFile = document.getElementById("reply-file")),
                (this.qrLblDrawing = document.getElementById("qr-drawlabel")),
                (this.formInputNodes = eForm.getElementsByTagName("input")),
                (this.qrInputNodes = eQr.getElementsByTagName("input")),
                tForm[0].addEventListener("click", this.onDrawClick, !1),
                tForm[1].addEventListener("click", this.onCancel, !1)),
                tQr[0].addEventListener("click", this.onDrawClick, !1),
                tQr[1].addEventListener("click", this.onCancel, !1);
    },
    onDrawClick: function() {
        var e,
            t,
            n = this.parentNode.getElementsByTagName("input");
        (e = +n[0].value),
        (t = +n[1].value),
        e < 1 ||
            t < 1 ||
            (Tegaki.open({
                onDone: PainterCore.onDone,
                onCancel: PainterCore.onCancel,
                saveReplay: PainterCore.formCbReplay && PainterCore.formCbReplay.checked,
                width: e,
                height: t
            }));
    },
    onDone: function() {
        var e, t, drawing, container;
        (e = PainterCore),
        (e.formBtnClear.disabled = !1),
        (e.qrBtnClear.disabled = !1),
        //TODO: When fchannel supports multiple attachments also attach replay
        //also include the drawing time
        Tegaki.flatten().toBlob((blob) => {
                drawing = new File([blob], "tegaki.png", {
                    type: "image/png",
                    lastModified: new Date().getTime()
                });
                container = new DataTransfer();
                container.items.add(drawing);
                e.formBtnFile.files = container.files;
                e.qrBtnFile.files = container.files;
            }),
            //Tegaki.saveReplay && (e.replayBlob = Tegaki.replayRecorder.toBlob()),
            !Tegaki.hasCustomCanvas && Tegaki.startTimeStamp ? (e.time = Math.round((Date.now() - Tegaki.startTimeStamp) / 1e3)) : (e.time = 0),
            (e.formBtnFile.style.visibility = "hidden"),
            (e.formLblDrawing.style.display = ""),
            (e.formBtnDraw.textContent = "Edit");
        (e.qrBtnFile.style.visibility = "hidden"),
        (e.qrLblDrawing.style.display = ""),
        (e.qrBtnDraw.textContent = "Edit");
        for (t of e.formInputNodes) t.disabled = !0;
        for (t of e.qrInputNodes) t.disabled = !0;
    },
    onCancel: function() {
        var e = PainterCore;
        (e.data = null), (e.replayBlob = null), (e.time = 0), (e.formBtnClear.disabled = !0), (e.formBtnFile.style.visibility = ""), (e.formLblDrawing.style.display = "none"), (e.formBtnDraw.textContent = "Draw"),
        (e.qrBtnClear.disabled = !0), (e.qrBtnFile.style.visibility = ""), (e.qrLblDrawing.style.display = "none"), (e.qrBtnDraw.textContent = "Draw");
        for (t of e.formInputNodes) t.disabled = !1;
        for (t of e.qrInputNodes) t.disabled = !1;
    },
};

function EditImage(imgUrl) {
    img = new Image();
    img.onload = Tegaki.onOpenImageLoaded;
    img.onerror = Tegaki.onOpenImageError;
    img.src = imgUrl;
    Tegaki.open({
        onDone: PainterCore.onDone,
        onCancel: PainterCore.onCancel,
        width: 500,
        height: 500,
    });
};

window.Tegaki && PainterCore.init()
