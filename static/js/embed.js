/*TODO: Settings menu (save config)
*/
const defythost = "https://www.youtube-nocookie.com"
var Config = {
	embedYT: true,
//	ytHost: "https://www.youtube-nocookie.com",
	embedSC: true,
	embedNND: true
};

Config.load = function() {
	var storage;
	
	if (storage = localStorage.getItem('fchan')) {
	  storage = JSON.parse(storage);
	  $.extend(Config, storage);
	}
  };

var getYTHost = function() {
	if (localStorage.getItem("ythost") === null) {
		return ('; ' + document.cookie).split(`; ythost=`).pop().split(';')[0] || defythost
	} else {
		return localStorage.getItem("ythost")
	}
}

var getInvQuality = function() {
	if (localStorage.getItem("ythost_quality") === null) {
		return ('; ' + document.cookie).split(`; ythost_quality=`).pop().split(';')[0] || "hd720"
	} else {
		return localStorage.getItem("ythost_quality")
	}
}


var setYTHost = function() {
	if (window.localStorage) {
		var ythost = localStorage.getItem("ythost");
		ythost = prompt("Youtube embed domain\n(https://youtube.com or Invidious instance e.g. https://yewtu.be)", getYTHost());
		localStorage.setItem("ythost", ythost || defythost);
	} else {
		var ythost = ('; ' + document.cookie).split(`; ythost=`).pop().split(';')[0] || defythost;
		ythost = prompt("Youtube embed domain\n(https://youtube.com or Invidious instance e.g. https://yewtu.be)", getYTHost());
		document.cookie = "ythost=" + ythost || defythost + "; expires=Fri, 31 Dec 9999 23:59:59 GMT";
	}

	if (window.localStorage) {
		var ythost_quality = localStorage.getItem("ythost_quality");
		ythost_quality = prompt("Invidious embed quality\n(dash, hd720, medium)", ythost_quality || "hd720");
		localStorage.setItem("ythost_quality", ythost_quality || "hd720");
	} else {
		var ythost_quality = ('; ' + document.cookie).split(`; ythost_quality=`).pop().split(';')[0] || "hd720";
		ythost_quality = prompt("Invidious embed quality\n(dash, hd720, medium)", ythost_quality || "hd720");
		document.cookie = "ythost_quality=" + ythost_quality || defythost + "; expires=Fri, 31 Dec 9999 23:59:59 GMT";
	}

}

if (Config.embedYT) {
ythost = getYTHost();
var ytembed = function(element) {
	id = element.getAttribute('data');
	if (element.textContent == 'Remove') {
		element.parentNode.removeChild(element.nextElementSibling);
		element.textContent = 'Embed';
	} else {
		el = document.createElement('div');
		el.className = 'media-embed';
		el.innerHTML = '<iframe src="' + getYTHost() + '/embed/' +
			id + '?quality=' + getInvQuality() + 
			'" width="854" height="480" frameborder="0" allowfullscreen style="max-width:100%;max-height:100%;"></iframe>';

		element.parentNode.insertBefore(el, element.nextElementSibling);

		element.textContent = 'Remove';
	}
	return false
};
}

if (Config.embedSC) {
var scembed = function(element) {
	var xhr, url;

	url = element.getAttribute('data');
	if (element.textContent == 'Remove') {
		element.parentNode.removeChild(element.nextElementSibling);
		element.textContent = 'Embed';
	} else if (element.textContent == 'Embed')  {
		xhr = new XMLHttpRequest();
		xhr.open('GET', 'https://soundcloud.com/oembed?show_artwork=false&'
		  + 'maxwidth=500px&show_comments=false&format=json&url='
		  + 'https://' + url);
		xhr.onload = function() {
			if (this.status == 200 || this.status == 304) {
				el = document.createElement('div');
				el.className = 'media-embed';
				el.innerHTML = JSON.parse(this.responseText).html;
				element.parentNode.insertBefore(el, element.nextElementSibling);
				element.textContent = 'Remove';
			} else {
				element.textContent = 'Error';
				console.log('SoundCloud Error (HTTP ' + this.status + ')');
			}
		};
			element.textContent = 'Loading...';
			xhr.send(null);
		};
	return false
};
}

if (Config.embedNND) {
	var nndembed = function(element) {
		id = element.getAttribute('data');
		if (element.textContent == 'Remove') {
			element.parentNode.removeChild(element.nextElementSibling);
			element.textContent = 'Embed';
		} else {
			el = document.createElement('div');
			el.className = 'media-embed';
			el.innerHTML = '<iframe src="https://embed.nicovideo.jp/watch/' +
				id +'" width="854" height="480" frameborder="0" allowfullscreen style="max-width:100%;max-height:100%;"></iframe>';
	
			element.parentNode.insertBefore(el, element.nextElementSibling);
	
			element.textContent = 'Remove';
		}
		return false
	};
	}



document.querySelectorAll('.comment').forEach(function(element) {
	if (Config.embedYT) {
		const ytregExp = /((?:watch\?v=|youtu\.be|youtube.com\/(?:shorts|v|live)\/)([\w-]{11})(?!\")(?:\S*))/g;
		element.innerHTML = element.innerHTML.replace(ytregExp, "$1 <span>[<a href='" + ythost + "/watch?v=$2' data='$2' onclick='return ytembed(this)'>Embed</a>]</span>");
	}
	if (Config.embedSC) {
		const scregExp = /((?:(?:on.)?soundcloud\.com|snd\.sc)\/[^\s<]+(?:<wbr>)?[^\s<]*)/g;
		element.innerHTML = element.innerHTML.replace(scregExp, "$1 <span>[<a href='https://$1' data='$1' onclick='return scembed(this)'>Embed</a>]</span>");
	}
	if (Config.embedNND) {
		const nndregExp = /(nicovideo\.jp\/watch\/((sm|so|lv)\d+))/g;
		element.innerHTML = element.innerHTML.replace(nndregExp, "$1 <span>[<a href='https://$1' data='$2' onclick='return nndembed(this)'>Embed</a>]</span>");
	}
});
