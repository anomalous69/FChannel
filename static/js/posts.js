/* TODO: better function names */
function hide(el) {
    id = el.id.replace('hidebtn-','')
    if (localStorage.getItem("hide") === null) {
        var ids = [];
    } else {
        var ids = JSON.parse(localStorage.getItem("hide"));
    }

    ids.push(id);
    localStorage.setItem("hide", JSON.stringify(ids));
    hidePost(id);
}

function unhide(el) {
    id = el.id.replace('hidebtn-','')
    if (localStorage.getItem("hide") === null) {
        var ids = [];
    } else {
        var ids = JSON.parse(localStorage.getItem("hide"));
    }

    ids = ids.filter(elem => elem !== id);
    localStorage.setItem("hide", JSON.stringify(ids));
    unhidePost(id);
}

function hidePost(id) {
    /* This can probably get pretty slow, posts should probably be wrapped with a div  */
    /* Also move this into a better file*/
    content = document.getElementById(id + "-content");
    if (content) {
    content.style.display = "none";
    content.parentElement.classList.add("postHidden");
    hidebtn = document.getElementById("hidebtn-"+id);
    if (hidebtn) {hidebtn.text = "Unhide post";
    hidebtn.onclick = function() {unhide(this)};}
    if (content) {content.style.display = "none";}
    finfo = document.getElementById(id + "-fileinfo");
    if (finfo) {finfo.style.display = "none";}
    attach = document.getElementById("media-" + id);
    if (attach) {attach.style.display = "none";}
    }
}

function unhidePost(id) {
    content = document.getElementById(id + "-content");
    if (content) {content.style.display = "block";
    content.parentElement.classList.remove("postHidden");}
    hidebtn = document.getElementById("hidebtn-"+id);
    if (hidebtn) {hidebtn.text = "Hide post";
    hidebtn.onclick = function() {hide(this)};}
    if (content) {content.style.display = "block";}
    finfo = document.getElementById(id + "-fileinfo");
    if (finfo) {finfo.style.display = "block";}
    attach = document.getElementById("media-" + id);
    if (attach) {attach.style.display = "block";}
}

if (localStorage.getItem("hide") !== null) {
    var ids = JSON.parse(localStorage.getItem("hide"));
    let i = 0;
    while (i < ids.length) {
        hidePost(ids[i])
        i++;
    }
}

function startNewPost() {
    var el = document.getElementById("newpostbtn");
    el.style = "display:none;";
    el.setAttribute("state", "1");
    drawform = document.getElementById("drawform")
    if (drawform) {
        drawform.style = "";
    }
    document.getElementById("newpost").style = "";
    document.getElementById("stopTablePost").style = "display:unset;";
    sessionStorage.setItem("newpostState", true);
}

function stopNewPost() {
    var el = document.getElementById("newpostbtn");
    el.style = "display:block;margin-bottom:100px;";
    el.setAttribute("state", "0");
    document.getElementById("newpost").style = "display: none;";
    sessionStorage.setItem("newpostState", false);
}

function shortURL(actorName, url) {
    re = /.+\//g;
    temp = re.exec(url);

    var output;

    if (stripTransferProtocol(temp[0]) == stripTransferProtocol(actorName) + "/") {
        var short = url.replace("https://", "");
        short = short.replace("http://", "");
        short = short.replace("www.", "");

        var re = /^.{3}/g;

        var u = re.exec(short);

        re = /\w+$/g;

        output = re.exec(short);
    } else {
        var short = url.replace("https://", "");
        short = short.replace("http://", "");
        short = short.replace("www.", "");

        var re = /^.{3}/g;

        var u = re.exec(short);

        re = /\w+$/g;

        u = re.exec(short);

        str = short.replace(/\/+/g, " ");

        str = str.replace(u, " ").trim();

        re = /(\w|[!@#$%^&*<>])+$/;

        v = re.exec(str);

        output = "f" + v[0] + "-" + u
    }

    return output;
}

function getBoardId(url) {
    var re = /\/([^/\n]+)(.+)?/gm;
    var matches = re.exec(url);
    return matches[1];
}

function convertContent(actorName, content, opid) {
    var re = /(>>)(https?:\/\/)?(www\.)?.+\/\w+/gm;
    var match = content.match(re);
    var newContent = content;
    if (match) {
        match.forEach(function (quote, i) {
            var link = quote.replace('>>', '');
            var isOP = "";
            if (link == opid) {
                isOP = " (OP)";
            }

            var q = link;

            if (document.getElementById(link + "-content") != null) {
                q = document.getElementById(link + "-content").innerText;
                q = q.replaceAll('>', '/\>');
                q = q.replaceAll('"', '');
                q = q.replaceAll("'", "");
            }
            newContent = newContent.replace(quote, '<a class="reply" title="' + q + '" href="' + (actorName) + "/" + shortURL(actorName, opid) + '#' + shortURL(actorName, link) + '";">>>' + shortURL(actorName, link) + isOP + '</a>');

        });
    }

    re = /^(\s+)?>.+/gm;

    match = newContent.match(re);
    if (match) {
        match.forEach(function (quote, i) {

            newContent = newContent.replace(quote, '<span class="quote">' + quote + '</span>');
        });
    }

    return newContent.replaceAll('/\>', '>');
}

function convertContentNoLink(actorName, content, opid) {
    var re = /(>>)(https?:\/\/)?(www\.)?.+\/\w+/gm;
    var match = content.match(re);
    var newContent = content;
    if (match) {
        match.forEach(function (quote, i) {
            var link = quote.replace('>>', '');
            var isOP = "";
            if (link == opid) {
                isOP = " (OP)";
            }

            var q = link;

            if (document.getElementById(link + "-content") != null) {
                q = document.getElementById(link + "-content").innerText;
            }

            newContent = newContent.replace(quote, '>>' + shortURL(actorName, link) + isOP);
        });
    }
    newContent = newContent.replaceAll("'", "");
    return newContent.replaceAll('"', '');
}

function setdeletionPassword(form) {
    localStorage.setItem("deletionPassword", form.pwd.value);
}

function getdeletionPassword() {
    passwordFields = document.getElementsByName("pwd");
    for (let i = 0; i < passwordFields.length; i++) {
        passwordFields[i].value = localStorage.getItem("deletionPassword");
    }
}

function closeReply() {
    document.getElementById("reply-box").style.display = "none";
    document.getElementById("reply-comment").value = "";

    sessionStorage.setItem("element-closed-reply", true);
}

function closeReport() {
    document.getElementById("report-box").style.display = "none";
    document.getElementById("report-comment").value = "";

    sessionStorage.setItem("element-closed-report", true);
}

function timeSince() {
    var delta, count, head, tail;
    timestamp = this.dataset.utc;
    
    delta = Date.now() / 1000 - timestamp;
    
    if (delta < 1) {
        return this.title = 'moments ago';
    }
    
    if (delta < 60) {
        return this.title = (0 | delta) + ' seconds ago';
    }
    
    if (delta < 3600) {
      count = 0 | (delta / 60);
      
      if (count > 1) {
        return this.title = count + ' minutes ago';
      }
      else {
        return this.title = 'one minute ago';
      }
    }
    
    if (delta < 86400) {
      count = 0 | (delta / 3600);
      
      if (count > 1) {
        head = count + ' hours';
      }
      else {
        head = 'one hour';
      }
      
      tail = 0 | (delta / 60 - count * 60);
      
      if (tail > 1) {
        head += ' and ' + tail + ' minutes';
      }
      
      return this.title = head + ' ago';
    }
    
    count = 0 | (delta / 86400);
    
    if (count > 1) {
      head = count + ' days';
    }
    else {
      head = 'one day';
    }
    
    tail = 0 | (delta / 3600 - count * 24);
    
    if (tail > 1) {
      head += ' and ' + tail + ' hours';
    }
    
    return this.title = head + ' ago';
  };

function quote(actorName, opid, id) {
    sessionStorage.setItem("element-closed-reply", false);
    var box = document.getElementById("reply-box");
    var header = document.getElementById("reply-header");
    var header_text = document.getElementById("reply-header-text");
    var comment = document.getElementById("reply-comment");
    var inReplyTo = document.getElementById("inReplyTo-box");

    var w = window.innerWidth / 2 - 200;
    var h = 300; //document.getElementById(id + "-content").offsetTop - 348;

    const boxStyle = "top: " + h + "px; left: " + w + "px;";
    box.setAttribute("style", boxStyle);
    sessionStorage.setItem("element-reply-style", boxStyle);
    sessionStorage.setItem("reply-top", h);
    sessionStorage.setItem("reply-left", w);


    if (inReplyTo.value != opid)
        comment.value = "";

    header_text.innerText = "Replying to Thread No. " + shortURL(actorName, opid);
    inReplyTo.value = opid;
    sessionStorage.setItem("element-reply-actor", actorName);
    sessionStorage.setItem("element-reply-id", inReplyTo.value);

    if (id != "reply")
        comment.value += ">>" + id + "\n";
    sessionStorage.setItem("element-reply-comment", comment.value);

    dragElement(header);
}

function report(actorName, id) {
    sessionStorage.setItem("element-closed-report", false);
    var box = document.getElementById("report-box");
    var header = document.getElementById("report-header");
    var comment = document.getElementById("report-comment");
    var inReplyTo = document.getElementById("report-inReplyTo-box");

    var w = window.innerWidth / 2 - 200;
    var h = 300; //document.getElementById(id + "-content").offsetTop - 348;

    const boxStyle = "top: " + h + "px; left: " + w + "px;";
    box.setAttribute("style", boxStyle);
    sessionStorage.setItem("element-report-style", boxStyle);
    sessionStorage.setItem("report-top", h);
    sessionStorage.setItem("report-left", w);

    header.innerText = "Report Post No. " + shortURL(actorName, id);
    inReplyTo.value = id;
    sessionStorage.setItem("element-report-actor", actorName);
    sessionStorage.setItem("element-report-id", id);

    dragElement(header);
}

var pos1, pos2, pos3, pos4;
var elmnt;

function closeDragElement(e) {
    // stop moving when mouse button is released:
    document.onmouseup = null;
    document.onmousemove = null;
    sessionStorage.setItem("eventhandler", false);
}

function elementDrag(e) {
    e = e || window.event;
    e.preventDefault();

    // calculate the new cursor position:
    pos1 = pos3 - e.clientX;
    pos2 = pos4 - e.clientY;
    pos3 = e.clientX;
    pos4 = e.clientY;

    sessionStorage.setItem("pos1", pos1);
    sessionStorage.setItem("pos2", pos2);
    sessionStorage.setItem("pos3", pos3);
    sessionStorage.setItem("pos4", pos4);

    // set the element's new position:
    var parent = elmnt.parentElement;
    var parentRect = parent.getBoundingClientRect();
    var parentTop = parentRect.top;
    var parentLeft = parentRect.left;
    var parentWidth = parentRect.width;
    var parentHeight = parentRect.height;
    var windowWidth = window.innerWidth;
    var windowHeight = window.innerHeight;
    var newTop = parentTop - pos2;
    var newLeft = parentLeft - pos1;

    // check if the element is going off the top or bottom of the screen
    if (newTop < 0) {
        newTop = 0;
    } else if (newTop + parentHeight > windowHeight) {
        newTop = windowHeight - parentHeight;
    }

    // check if the element is going off the left or right of the screen
    if (newLeft < 0) {
        newLeft = 0;
    } else if (newLeft + parentWidth > windowWidth) {
        newLeft = windowWidth - parentWidth;
    }

    parent.style.top = newTop + "px";
    parent.style.left = newLeft + "px";

    if (elmnt.id.startsWith("report")) {
        sessionStorage.setItem("report-top", parent.style.top);
        sessionStorage.setItem("report-left", parent.style.left);
    } else if (elmnt.id.startsWith("reply")) {
        sessionStorage.setItem("reply-top", parent.style.top);
        sessionStorage.setItem("reply-left", parent.style.left);
    }
}

function dragMouseDown(e) {
    e = e || window.event;
    e.preventDefault();

    // get the mouse cursor position at startup:
    pos3 = e.clientX;
    pos4 = e.clientY;
    sessionStorage.setItem("pos3", pos3);
    sessionStorage.setItem("pos4", pos4);

    elmnt = e.currentTarget;

    // call a function whenever the cursor moves:
    document.onmouseup = closeDragElement;
    document.onmousemove = elementDrag;
    sessionStorage.setItem("eventhandler", true);

}

function dragElement(elmnt) {
    elmnt.onmousedown = dragMouseDown;
}

const stateLoadHandler = function (event) {
    pos1 = parseInt(sessionStorage.getItem("pos1"));
    pos2 = parseInt(sessionStorage.getItem("pos2"));
    pos3 = parseInt(sessionStorage.getItem("pos3"));
    pos4 = parseInt(sessionStorage.getItem("pos4"));

    if (sessionStorage.getItem("element-closed-report") === "false") {
        var box = document.getElementById("report-box");
        var header = document.getElementById("report-header");
        var comment = document.getElementById("report-comment");
        var inReplyTo = document.getElementById("report-inReplyTo-box");

        header.onmousedown = dragMouseDown;
        inReplyTo.value = parseInt(sessionStorage.getItem("element-report-id"));
        header.innerText = "Report Post No. " + shortURL(sessionStorage.getItem("element-report-actor"), sessionStorage.getItem("element-report-id"));
        comment.value = sessionStorage.getItem("element-report-comment");

        box.setAttribute("style", sessionStorage.getItem("element-report-style"));

        box.style.top = sessionStorage.getItem("report-top");
        box.style.left = sessionStorage.getItem("report-left");

        if (sessionStorage.getItem("eventhandler") === "true") {
            elmnt = header;
            document.onmouseup = closeDragElement;
            document.onmousemove = elementDrag;
        } else {
            document.onmouseup = null;
            document.onmousemove = null;
        }
    }
    if (sessionStorage.getItem("element-closed-reply") === "false") {
        var box = document.getElementById("reply-box");
        var header = document.getElementById("reply-header");
        var header_text = document.getElementById("reply-header-text");
        var comment = document.getElementById("reply-comment");
        var inReplyTo = document.getElementById("inReplyTo-box");

        header.onmousedown = dragMouseDown;
        inReplyTo.value = parseInt(sessionStorage.getItem("element-reply-id"));
        header_text.innerText = "Replying to Thread No. " + shortURL(sessionStorage.getItem("element-reply-actor"), sessionStorage.getItem("element-reply-id"));
        comment.value = sessionStorage.getItem("element-reply-comment");

        pos1 = parseInt(sessionStorage.getItem("pos1"));
        pos2 = parseInt(sessionStorage.getItem("pos2"));
        pos3 = parseInt(sessionStorage.getItem("pos3"));
        pos4 = parseInt(sessionStorage.getItem("pos4"));

        box.setAttribute("style", sessionStorage.getItem("element-reply-style"));

        box.style.top = sessionStorage.getItem("reply-top");
        box.style.left = sessionStorage.getItem("reply-left");

        if (sessionStorage.getItem("eventhandler") === "true") {
            elmnt = header;
            document.onmouseup = closeDragElement;
            document.onmousemove = elementDrag;
        } else {
            document.onmouseup = null;
            document.onmousemove = null;
        }
    }
};

document.addEventListener("DOMContentLoaded", stateLoadHandler, false);

function stripTransferProtocol(value) {
    var re = /(https:\/\/|http:\/\/)?(www.)?/;
    return value.replace(re, "");
}

if (localStorage.getItem("deletionPassword") !== null) {
    getdeletionPassword();
}
else {
    localStorage.setItem("deletionPassword", Math.random().toString(36).slice(-14));
    getdeletionPassword();
}

document.querySelectorAll('.timestamp').forEach(timestamp => {
  timestamp.addEventListener('mouseover', timeSince);
});