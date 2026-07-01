import { CodeJar } from './codejar.min.js';

const highlight = editor => {
    let code = editor.textContent;
    // Escape HTML
    code = code.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    
    // Define groups
    const jumps = /\b(if|elif|else|while|endwhile|break|exit|endif)\b/g;
    const keywords = /\b(connect|listen|disconnect|reset|seq|send|wait|expect|loadmsg|set|incr|decr|print|sleep|assert|include|waitstatus)\b/g;
    
    // Apply highlighting
    // Using a placeholder approach prevents "double-highlighting" if regexes overlapped
    code = code.replace(jumps, '<span class="text-purple-400 font-bold">$1</span>');
    code = code.replace(keywords, '<span class="text-blue-400 font-bold">$1</span>');
    code = code.replace(/(#.*)/g, '<span class="text-gray-500">$1</span>');
    code = code.replace(/(\$[\w\.\[\]]+)/g, '<span class="text-yellow-400">$1</span>');
    
    editor.innerHTML = code;
};

export async function initCodeJar(editorDiv, hiddenInput) {
    try {
    const jar = CodeJar(editorDiv, highlight, { tab: '    ' });

    jar.onUpdate(code => { 
        hiddenInput.value = code; 
        localStorage.setItem("mxshell-script", code);
    });
    
    // Load initial state from storage or default
    const saved = localStorage.getItem("mxshell-script") || hiddenInput.value;
    jar.updateCode(saved);
    
    } catch (error) {
        console.error("CodeJar Init Error:", error);
        editorDiv.innerHTML = `<span class="text-red-500">Editor failed to load.</span>`;
    }
}
