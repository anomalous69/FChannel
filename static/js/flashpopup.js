window.RufflePlayer = window.RufflePlayer || {};

let settings = {
	autoplay: "auto",
	allowScriptAccess: false,
	warnOnUnsupportedContent: false,
	letterbox: 'on',
	splashScreen: false
};

function streamedFetch(url) {
	console.log('streamedFetch');

	showStatus();

	let loadedBytes = 0;

	return fetch(url)
		.then(resp => {
			let totalBytes = +resp.headers.get('Content-Length');

			const reader = resp.body.getReader();

			return new ReadableStream({
				start(controller) {
					function read() {
						reader.read().then(({
							done,
							value
						}) => {
							if (done) {
								setStatus('Loading 100%');
								controller.close();
								return;
							}
							controller.enqueue(value);
							loadedBytes += value.byteLength;
							setStatus('Loading ' + (~~(loadedBytes / totalBytes * 100)) + '%');
							read();
						})
					}

					read();
				}
			});
		})
		.then(stream => {
			setStatus('Buffering…');
			return new Response(stream).arrayBuffer();
		});
}

function setStatus(str) {
	document.getElementById('state').textContent = str;
}

function showStatus() {
	document.getElementById('state').style.display = "";
}

function hideStatus() {
	document.getElementById('state').style.display = "none";
}

function playSWF(player, settings) {
	setStatus('Loading player…');
	player.load(settings)
		.then(() => {
			hideStatus();
			margins = 10;
			headerHeight = 20;
			cntWidth = player.metadata.width;
			cntHeight = player.metadata.height;
			docWidth = document.documentElement.clientWidth;
			docHeight = document.documentElement.clientHeight;
			maxWidth = docWidth - margins;
			maxHeight = docHeight - margins - headerHeight;
			ratio = cntWidth / cntHeight;
			container = document.querySelector("#swf-embed > div");

			if (cntWidth > maxWidth) {
				cntWidth = maxWidth;
				cntHeight = Math.round(maxWidth / ratio);
			}

			if (cntHeight > maxHeight) {
				cntHeight = maxHeight;
				cntWidth = Math.round(maxHeight * ratio);
			}

			container.style.position = 'fixed';
			console.log(container);
			container.style.width = cntWidth + 'px';
			container.style.height = cntHeight + 'px';
			container.style.top = '50%';
			container.style.left = '50%';
			container.style.marginTop = (-cntHeight / 2 - headerHeight / 2) + 'px';
			container.style.marginLeft = (-cntWidth / 2) + 'px';

			document.getElementById("swf-embed-header-text").appendChild(document.createTextNode(", " + player.metadata.width + "x" + player.metadata.height))
			player.setAttribute('width', cntWidth);
			player.setAttribute('height', cntHeight);
			player.style.width = cntWidth + 'px';
			player.style.height = cntHeight + 'px';

		})
		.catch(() => {
			setStatus("Couldn't load the player");
		});
}

function swfpopup(e, boardtype) {
	let container = document.createElement('div');
	container.id = "swf-container";
	var url;
	var title;
	if (boardtype === "list") {
		url = e.parentElement.previousElementSibling.firstElementChild.href;
		title = e.parentElement.previousElementSibling.firstElementChild.download;
	}
	else if (boardtype === "image") {
		url = e.parentElement.firstElementChild.href;
		title = e.parentElement.firstElementChild.download;
	}
	document.getElementById("swf-embed-header-text").textContent = title;

	document.getElementById("swf-embed").firstChild.appendChild(container);

	let ruffle = window.RufflePlayer.newest();
	let player = ruffle.createPlayer();

	container.appendChild(player);

	document.getElementById("swf-embed").style.display = "";

	if (!!window.ReadableStream) {
		streamedFetch(url).then(data => {
			settings.data = new Uint8Array(data);
			playSWF(player, settings);
		})
	} else {
		settings.url = url;
		playSWF(player, settings);
	}
}
